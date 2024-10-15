#!/bin/bash
set -e

init_env(){
    cat <<EOF > $OVPN_DATA/pki/vars
EASYRSA_PKI=$OVPN_DATA/pki
EASYRSA_CA_EXPIRE=3650
EASYRSA_CERT_EXPIRE=3650
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
dev tun
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
client-disconnect "/usr/bin/docker-entrypoint.sh addhistory"
script-security 3
status $OVPN_DATA/openvpn-status.log
duplicate-cn
max-clients $OVPN_MAXCLIENTS
management ${OVPN_MANAGEMENT/:/ }
verb 2
setenv ovpn_data ${OVPN_DATA:-/data}
setenv auth_api ${AUTH_API:-http://127.0.0.1/login}
setenv ovpn_auth_api ${OVPN_AUTH_API:-http://127.0.0.1/ovpn/login}
setenv ovpn_history_api ${OVPN_HISTORY_API:-http://127.0.0.1:8833/ovpn/history}
setenv auth_token $(echo "$ADMIN_USERNAME:$ADMIN_PASSWORD" | openssl enc -e -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY)
EOF
}

run_server(){
    mkdir -p /dev/net
    if [ ! -c /dev/net/tun ]; then
        mknod /dev/net/tun c 10 200
    fi

    ipt="iptables-nft"
    if iptables-legacy -L -n -t nat > /dev/null 2>&1; then
        ipt="iptables-legacy"
    fi

    $ipt -t nat -C POSTROUTING -s $OVPN_SUBNET -j MASQUERADE > /dev/null 2>&1 || {
        $ipt -t nat -A POSTROUTING -s $OVPN_SUBNET -j MASQUERADE
    }

    /usr/sbin/openvpn $OVPN_DATA/server.conf
}

update_config(){
    source $OVPN_DATA/.vars

    config=$OVPN_DATA/server.conf
    auth_api=$(grep '^setenv auth_api' $config | cut -d' ' -f3)
    ovpn_auth_api=$(grep '^setenv ovpn_auth_api' $config | cut -d' ' -f3)
    ovpn_history_api=$(grep '^setenv ovpn_history_api' $config | cut -d' ' -f3)
    auth_token=$(grep '^setenv auth_token' $config | cut -d' ' -f3)
    ovpn_data=$(grep '^setenv ovpn_data' $config | cut -d' ' -f3)
    ovpn_subnet=$(grep '^server' $config | cut -d' ' -f2,3)
    ovpn_maxclients=$(grep '^max-clients' $config | cut -d' ' -f2)
    ovpn_proto=$(grep '^proto' $config | cut -d' ' -f2)
    ovpn_port=$(grep '^port' $config | cut -d' ' -f2)
    ovpn_management=$(grep '^management' $config | cut -d' ' -f2,3)

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

    if [ "$ovpn_history_api" != "$OVPN_HISTORY_API" ]; then
        if [ -z "$ovpn_history_api" ]; then
            echo "setenv ovpn_history_api $OVPN_HISTORY_API" >> $config
        else
            sed -i "s|^setenv ovpn_history_api .*|setenv ovpn_history_api $OVPN_HISTORY_API|" $config
        fi
    fi

    decrypt_auth_token=$(echo "$auth_token" | openssl enc -d -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY 2>/dev/null || true)
    if [ "$decrypt_auth_token" != "$ADMIN_USERNAME:$ADMIN_PASSWORD" ]; then
        AUTH_TOKEN=$(echo "$ADMIN_USERNAME:$ADMIN_PASSWORD" | openssl enc -e -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY)
        if [ -z "$auth_token" ]; then
            echo "setenv auth_token $AUTH_TOKEN" >> $config
        else
            sed -i "s|^setenv auth_token .*|setenv auth_token $AUTH_TOKEN|" $config
        fi
    fi

    if [ "$ovpn_data" != "$OVPN_DATA" ]; then
        if [ -z "$ovpn_data" ]; then
            echo "setenv ovpn_data $OVPN_DATA" >> $config
        else
            sed -i "s|^setenv ovpn_data .*|setenv ovpn_data $OVPN_DATA|" $config 
        fi
    fi


    if [ "$ovpn_subnet" != "$(getsubnet $OVPN_SUBNET)" ]; then
        if [ -z "$ovpn_subnet" ]; then
            echo "server $(getsubnet $OVPN_SUBNET)" >> $config
        else
            sed -i "s|^server .*|server $(getsubnet $OVPN_SUBNET)|" $config 
        fi
    fi

    if [ "$ovpn_maxclients" != "$OVPN_MAXCLIENTS" ]; then
        if [ -z "$ovpn_maxclients" ]; then
            echo "max-clients $OVPN_MAXCLIENTS" >> $config
        else
            sed -i "s|^max-clients .*|max-clients $OVPN_MAXCLIENTS|" $config 
        fi
    fi

    if [ "$ovpn_proto" != "$OVPN_PROTO" ]; then
        if [ -z "$ovpn_proto" ]; then
            echo "proto $OVPN_PROTO" >> $config
        else
            sed -i "s|^proto .*|proto $OVPN_PROTO|" $config 
        fi
    fi

    if [ "$ovpn_port" != "$OVPN_PORT" ]; then
        if [ -z "$ovpn_port" ]; then
            echo "port $OVPN_PORT" >> $config
        else
            sed -i "s|^port .*|port $OVPN_PORT|" $config 
        fi
    fi

    if [ "$ovpn_management" != "${OVPN_MANAGEMENT/:/ }" ]; then
        if [ -z "$ovpn_management" ]; then
            echo "management ${OVPN_MANAGEMENT/:/ }" >> $config
        else
            sed -i "s|^management .*|management ${OVPN_MANAGEMENT/:/ }|" $config 
        fi
    fi
}

renew_cert(){
    source $OVPN_DATA/.vars
    source $OVPN_DATA/pki/vars

    cd $OVPN_DATA/pki
    openssl x509 -in ca.crt -days $EASYRSA_CA_EXPIRE -out ca.crt -signkey private/ca.key
    /usr/share/easy-rsa/easyrsa --batch renew $SERVER_NAME
    /usr/share/easy-rsa/easyrsa --batch revoke-renewed $SERVER_NAME
    /usr/share/easy-rsa/easyrsa gen-crl
}

auth(){
    if [ "$1" = "true" ]; then
        sed -i 's/^#auth-user-pass-verify/auth-user-pass-verify/' $OVPN_DATA/server.conf
    else
        sed -i 's/^auth-user-pass-verify/#&/' $OVPN_DATA/server.conf
    fi
}

getsubnet(){
    ip=$(echo $1 | cut -d'/' -f1)
    prefix=$(echo $1 | cut -d'/' -f2)

    mask=""
    for i in {1..4}; do
        if [ $prefix -ge 8 ]; then
            mask+="255"
            prefix=$((prefix - 8))
        else
            mask+=$((256 - 2**(8 - prefix)))
            prefix=0
        fi

        if [ $i -lt 4 ]; then
            mask+="."
        fi
    done
    echo $ip $mask
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
$(grep -q '^auth-user-pass-verify' $OVPN_DATA/server.conf && echo 'auth-user-pass' || echo '#auth-user-pass')
cipher AES-128-GCM
tls-client
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
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

add_history(){
    #https://build.openvpn.net/man/openvpn-2.6/openvpn.8.html#environmental-variables
    auth=$(source $ovpn_data/.vars && echo $auth_token|openssl enc -d -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY)
    IFS=':' read -r user pass <<< $auth
    response=$(curl --connect-timeout 5 -s -D - -o /dev/null -d "username=$user&password=$pass" $auth_api)
    cookie=$(echo $response | awk -F 'Set-Cookie: ' '{print $2}' | awk '{print $1}')
    data="vip=$ifconfig_pool_remote_ip&rip=$trusted_ip&common_name=$common_name&username=$username&bytes_received=$bytes_received&bytes_sent=$bytes_sent&time_unix=$time_unix&time_duration=$time_duration"

    status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -b $cookie -d $data $ovpn_history_api)

    [ $status -ne 200 ] && echo "[CLIENT-DISCONNECT] $0:$LINENO 保存历史记录出错，请检查！" || true
}

################################################################################################

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
    "auth")
        $(auth $2)

        supervisorctl stop openvpn && sleep 1 && supervisorctl start openvpn
        exit 0
    ;;
    "renewcert")
        renew_cert

        supervisorctl stop openvpn && sleep 1 && supervisorctl start openvpn
        exit 0
    ;;
    "addhistory")
        set +e
        add_history
        [ $? -ne 0 ] && echo "[CLIENT-DISCONNECT] $0:$LINENO 保存历史记录出错，请检查！"
        set -e
        
        exit 0
    ;;
    "/usr/sbin/openvpn")
        update_config
        run_server
    ;;
    "/usr/bin/supervisord")
        if [ ! -e $OVPN_DATA/.vars ]; then
            echo "请执行命令docker-compose run --rm openvpn --init进行初始化配置！"
            exit 1
        fi
        /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
    ;;
esac

exec "$@"
