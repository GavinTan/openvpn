#!/bin/bash

set -e


init_env(){
    cat <<EOF > $OVPN_DATA/pki/vars
EASYRSA_PKI=$OVPN_DATA/pki
EASYRSA_CRL_DAYS=3650
EASYRSA_ALGO=ec
EASYRSA_CURVE=prime256v1
EOF
    [ ! -f "$OVPN_DATA/.vars" ] && cat <<EOF > $OVPN_DATA/.vars
SECRET_KEY=$(head /dev/urandom | tr -dc A-Za-z0-9 | fold -w 50 | head -n 1)
SERVER_NAME=server_$(head /dev/urandom | tr -dc A-Za-z0-9 | fold -w 16 | head -n 1)
SERVER_CN=ovpn_$(head /dev/urandom | tr -dc A-Za-z0-9 | fold -w 16 | head -n 1)
EOF
    source $OVPN_DATA/.vars
}

init_pki(){
    cd $OVPN_DATA && /usr/share/easy-rsa/easyrsa init-pki
    init_env
    /usr/share/easy-rsa/easyrsa --batch --req-cn="$SERVER_CN" build-ca nopass
    /usr/share/easy-rsa/easyrsa --batch build-server-full "$SERVER_NAME" nopass
    /usr/share/easy-rsa/easyrsa gen-crl
    /usr/sbin/openvpn --genkey secret $OVPN_DATA/pki/tc.key
}

init_config(){
    cat <<EOF > $OVPN_DATA/server.conf
port $OVPN_PORT
proto $OVPN_PROTO
dev tuncd 
persist-key
persist-tun
keepalive 10 120
topology subnet
server $(getsubnet $OVPN_SUBNET)
#push "dhcp-option DNS 1.1.1.1"
#push "dhcp-option DNS 8.8.8.8"
#push "redirect-gateway def1 bypass-dhcp"
dh none
tls-groups prime256v1
tls-crypt $OVPN_DATA/pki/tc.key
crl-verify $OVPN_DATA/pki/crl.pem
ca $OVPN_DATA/pki/ca.crt
cert $OVPN_DATA/pki/issued/$SERVER_NAME.crt
key $OVPN_DATA/pki/private/$SERVER_NAME.key
auth SHA256
cipher AES-128-GCM
data-ciphers AES-128-GCM
tls-server
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
explicit-exit-notify 1
auth-user-pass-verify /usr/lib/openvpn/plugins/openvpn-auth via-env
script-security 3
status $OVPN_DATA/openvpn-status.log
duplicate-cn
management 127.0.0.1 $OVPN_MANAGE_PORT
verb 2
setenv ovpn_data ${OVPN_DATA:-/data}
setenv auth_api ${AUTH_API:-http://127.0.0.1/login}
setenv ovpn_auth_api ${OVPN_AUTH_API:-http://127.0.0.1/ovpn/login}
setenv auth_token $(echo "$ADMIN_USERNAME:$ADMIN_PASSWORD" | openssl enc -e -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY)
EOF
}

run_server(){
    mkdir -p /dev/net
    if [ ! -c /dev/net/tun ]; then
        mknod /dev/net/tun c 10 200
    fi

    iptables-nft -t nat -C POSTROUTING -s $OVPN_SUBNET -j MASQUERADE > /dev/null 2>&1 || {
        iptables-nft -t nat -A POSTROUTING -s $OVPN_SUBNET -j MASQUERADE
    }

    /usr/sbin/openvpn $OVPN_DATA/server.conf
}

checkEnvUpdateConfig(){
    source $OVPN_DATA/.vars

    config=$OVPN_DATA/server.conf
    auth_api=$(grep '^setenv auth_api' $config | cut -d' ' -f3)
    ovpn_auth_api=$(grep '^setenv ovpn_auth_api' $config | cut -d' ' -f3)
    auth_token=$(grep '^setenv auth_token' $config | cut -d' ' -f3)
    AUTH_TOKEN=$(echo "$ADMIN_USERNAME:$ADMIN_PASSWORD" | openssl enc -e -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY)


    if [ "$auth_api" != "$AUTH_API" ]; then
        if [ -z "$auth_api" ]; then
            echo "setenv auth_api $AUTH_API" >> $config
        else
            sed -i "s|^setenv auth_api .*|setenv auth_api $AUTH_API|" $config 
        fi
    fi

    if [ "$ovpn_auth_api" != "$OVPN_AUTH_API" ]; then
        if [ -z "$ovpn_auth_api" ]; then
            echo "setenv ovpn_auth_api $OVPN_AUTH_API" >> $config
        else
            sed -i "s|^setenv ovpn_auth_api .*|setenv ovpn_auth_api $OVPN_AUTH_API|" $config
        fi
    fi

    set +e
    decrypt_auth_token=$(echo "$auth_token" | openssl enc -d -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY)
    if [ "$decrypt_auth_token" != "$ADMIN_USERNAME:$ADMIN_PASSWORD" ]; then
        if [ -z "$auth_token" ]; then
            echo "setenv auth_token $AUTH_TOKEN" >> $config
        else
            sed -i "s|^setenv auth_token .*|setenv auth_token $AUTH_TOKEN|" $config
        fi
    fi
    set -e
}

cidr2mask(){
    local i
    local subnetmask=""
    local cidr=${1#*/}
    local full_octets=$(($cidr/8))
    local partial_octet=$(($cidr%8))

    for ((i=0;i<4;i+=1)); do
        if [ $i -lt $full_octets ]; then
            subnetmask+=255
        elif [ $i -eq $full_octets ]; then
            subnetmask+=$((256 - 2**(8-$partial_octet)))
        else
            subnetmask+=0
        fi
        [ $i -lt 3 ] && subnetmask+=.
    done
    echo $subnetmask
}

getsubnet() {
    echo ${1%/*} $(cidr2mask $1)
}

genclient() {
    if [ ! -f "$EASYRSA_PKI/private/$1.key" ]; then
        /usr/share/easy-rsa/easyrsa --batch build-client-full $1 nopass > /dev/null
    fi
    mkdir -p $OVPN_DATA/clients
    cat <<EOF > $OVPN_DATA/clients/$1.ovpn
client
proto $OVPN_PROTO
explicit-exit-notify
remote ${2:-$(ip -4 route get 8.8.8.8 | awk {'print $7'} | tr -d '\n')} $OVPN_PORT
dev tun
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
verify-x509-name $SERVER_NAME name
auth SHA256
auth-nocache
$(egrep -q '^auth-user-pass-verify' $OVPN_DATA/server.conf && echo 'auth-user-pass' || echo '#auth-user-pass')
cipher AES-128-GCM
tls-client
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
#ignore-unknown-option block-outside-dns
#setenv opt block-outside-dns # Prevent Windows 10 DNS leak
verb 3

## Custom configuration ##
$(echo -e $3)
## end ##

<ca>
$(cat $OVPN_DATA/pki/ca.crt)
</ca>
<cert>
$(openssl x509 -in $OVPN_DATA/pki/issued/$1.crt)
</cert>
<key>
$(cat $OVPN_DATA/pki/private/$1.key)
</key>
<tls-crypt>
$(cat $OVPN_DATA/pki/tc.key)
</tls-crypt>
EOF
}

if [ "$1" == "--init" ]; then
    init_pki
    init_config
    exit 0
fi

if [ "${1#-}" != "$1" ]; then
    set -- /usr/sbin/openvpn "$@"
fi

case $1 in
    "genclient")
        if [ -z $2 ]; then
            echo "请输入生成客户端名称！"
            exit 1
        fi

        $(genclient $2 $3 "$4")
        exit 0
    ;;
    "/usr/sbin/openvpn")
        checkEnvUpdateConfig
        run_server
    ;;
    "/usr/bin/supervisord")
        /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
    ;;
esac

exec "$@"
