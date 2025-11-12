#!/bin/bash
set -e

SYSTEM_CONFIG="/data/config.json"

init_pki() {
    SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)
    SERVER_CN=$(jq -r '.system.base.server_cn // ""' $SYSTEM_CONFIG ) 
    cd $OVPN_DATA && /usr/share/easy-rsa/easyrsa init-pki
   
    cat << EOF > $OVPN_DATA/pki/vars
set_var EASYRSA $OVPN_DATA
set_var EASYRSA_CA_EXPIRE 365
set_var EASYRSA_CERT_EXPIRE 365
set_var EASYRSA_CRL_DAYS 365
set_var EASYRSA_ALGO ec
set_var EASYRSA_CURVE prime256v1
EOF

    /usr/share/easy-rsa/easyrsa --batch --req-cn="$SERVER_CN" build-ca nopass
    /usr/share/easy-rsa/easyrsa --batch build-server-full "$SERVER_NAME" nopass
    /usr/share/easy-rsa/easyrsa gen-crl
    /usr/sbin/openvpn --genkey secret $OVPN_DATA/pki/tc.key
}

init_config() {
    SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)
    OVPN_PORT=$(jq -r '.openvpn.ovpn_port // "1194"' $SYSTEM_CONFIG)
    OVPN_PROTO=$(jq -r '.openvpn.ovpn_proto // "udp"' $SYSTEM_CONFIG)
    OVPN_MAXCLIENTS=$(jq -r '.openvpn.ovpn_maxclients // "200"' $SYSTEM_CONFIG)
    OVPN_IPV6=$(jq -r '.openvpn.ovpn_ipv6 // "false"' $SYSTEM_CONFIG)
    OVPN_GATEWAY=$(jq -r '.openvpn.ovpn_gateway // "false"' $SYSTEM_CONFIG)
    OVPN_SUBNET=$(jq -r '.openvpn.ovpn_subnet // "10.8.0.0/24"' $SYSTEM_CONFIG)
    OVPN_SUBNET6=$(jq -r '.openvpn.ovpn_subnet6 // "fdaf:f178:e916:6dd0::/64"' $SYSTEM_CONFIG)

    cat << EOF > $OVPN_DATA/server.conf
port $OVPN_PORT
proto $OVPN_PROTO
dev tun
persist-key
persist-tun
keepalive 10 60
topology subnet
$([[ "$OVPN_IPV6" == "true" ]] && echo -e "server $(getsubnet $OVPN_SUBNET)\nserver-ipv6 $OVPN_SUBNET6" || echo "server $(getsubnet $OVPN_SUBNET)")
$([[ "$OVPN_GATEWAY" == "true" ]] && echo -e 'push "dhcp-option DNS 8.8.8.8"\npush "dhcp-option DNS 2001:4860:4860::8888"\npush "redirect-gateway def1 ipv6 bypass-dhcp"' || echo -e '#push "dhcp-option DNS 8.8.8.8"\n#push "dhcp-option DNS 2001:4860:4860::8888"\n#push "redirect-gateway def1 ipv6 bypass-dhcp"')
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
auth-user-pass-verify /usr/lib/openvpn/plugins/openvpn-auth via-env
client-disconnect /usr/bin/docker-entrypoint.sh
client-connect /usr/bin/docker-entrypoint.sh
script-security 3
status $OVPN_DATA/openvpn-status.log
client-config-dir $OVPN_DATA/ccd
duplicate-cn
client-to-client
max-clients $OVPN_MAXCLIENTS
management ${OVPN_MANAGEMENT/:/ }
verb 2
$([[ "$OVPN_PROTO" =~ "udp" ]]  && echo "explicit-exit-notify 1")
setenv ovpn_data ${OVPN_DATA:-/data}
setenv auth_api http://127.0.0.1:$WEB_PORT/login
setenv ovpn_auth_api http://127.0.0.1:$WEB_PORT/ovpn/login
setenv ovpn_history_api http://127.0.0.1:$WEB_PORT/ovpn/history
EOF
}

run_server() {
    mkdir -p /dev/net
    if [ ! -c /dev/net/tun ]; then
        mknod /dev/net/tun c 10 200
    fi

    ipt="iptables-nft"
    if iptables-legacy -L -n -t nat > /dev/null 2>&1; then
        ipt="iptables-legacy"
    fi

    config=$OVPN_DATA/server.conf

    ovpn_subnet=$(awk '$1=="server"{print $2, $3}' $config)
    $ipt -t nat -C POSTROUTING -s ${ovpn_subnet/ /\/} -j MASQUERADE > /dev/null 2>&1 || {
        $ipt -t nat -A POSTROUTING -s ${ovpn_subnet/ /\/} -j MASQUERADE
    }


    if jq -e '.openvpn.ovpn_ipv6' $SYSTEM_CONFIG > /dev/null 2>&1; then
        ovpn_subnet6=$(awk '$1=="server-ipv6"{print $2, $3}' $config)
        ${ipt/iptables/ip6tables} -t nat -C POSTROUTING -s $ovpn_subnet6 -j MASQUERADE > /dev/null 2>&1 || {
            ${ipt/iptables/ip6tables} -t nat -A POSTROUTING -s $ovpn_subnet6 -j MASQUERADE
        }
    fi

    /usr/sbin/openvpn $OVPN_DATA/server.conf
}

renew_cert() {
    init_env

    #cd $OVPN_DATA/pki
    #openssl x509 -in ca.crt -days $1 -out ca.crt -signkey private/ca.key
    /usr/share/easy-rsa/easyrsa --batch --days=$1 renew-ca
    /usr/share/easy-rsa/easyrsa --batch --days=$1 renew $SERVER_NAME
    /usr/share/easy-rsa/easyrsa --batch revoke-renewed $SERVER_NAME
    /usr/share/easy-rsa/easyrsa --batch --days=$1 gen-crl
}

auth() {
    if [ "$1" = "true" ]; then
        sed -i 's/^#auth-user-pass-verify/auth-user-pass-verify/' $OVPN_DATA/server.conf
    else
        sed -i 's/^auth-user-pass-verify/#&/' $OVPN_DATA/server.conf
    fi
}

getsubnet() {
    ip=$(echo $1 | cut -d'/' -f1)
    prefix=$(echo $1 | cut -d'/' -f2)

    mask=""
    for i in {1..4}; do
        if [ $prefix -ge 8 ]; then
            mask+="255"
            prefix=$((prefix - 8))
        else
            mask+=$((256 - 2 ** (8 - prefix)))
            prefix=0
        fi

        if [ $i -lt 4 ]; then
            mask+="."
        fi
    done
    echo $ip $mask
}

genclient() {
    SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)
    OVPN_PROTO=$(jq -r '.openvpn.ovpn_proto // "udp"' $SYSTEM_CONFIG)
    OVPN_PORT=$(jq -r '.openvpn.ovpn_port // "1194"' $SYSTEM_CONFIG)
    OVPN_IPV6=$(jq -r '.openvpn.ovpn_ipv6 // "false"' $SYSTEM_CONFIG)

    if [ ! -f "$EASYRSA_PKI/private/$1.key" ]; then
        /usr/share/easy-rsa/easyrsa --batch build-client-full $1 nopass > /dev/null
    fi
    mkdir -p $OVPN_DATA/clients
    cat << EOF > $OVPN_DATA/clients/$1.ovpn
client
proto $([[ "$OVPN_IPV6" == "true" ]] && [[ ! "$OVPN_PROTO" =~ 6 ]] && echo "${OVPN_PROTO}6" || echo $OVPN_PROTO)
remote ${2:-$([[ "$OVPN_IPV6" == "true" ]] && ip -6 route get 2001:4860:4860::8888 | grep -oP 'src \K\S+' || ip -4 route get 8.8.8.8 | grep -oP 'src \K\S+')} ${3:-$OVPN_PORT}
dev tun
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
verify-x509-name $SERVER_NAME name
auth SHA256
$(grep -q '^auth-user-pass-verify' $OVPN_DATA/server.conf && echo 'auth-user-pass' || echo '#auth-user-pass')
cipher AES-128-GCM
tls-client
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
verb 3
$([[ "$OVPN_IPV6" == "true" ]] && echo -e "tun-mtu 1400\nmssfix 1360")
$([[ "$OVPN_PROTO" =~ "udp" ]] && echo "explicit-exit-notify")
$([[ "$5" == "true" ]] && echo 'static-challenge "Enter MFA code" 1')

## Custom configuration ##
$(echo -e $4)
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

check_config() {
    config=$OVPN_DATA/server.conf
    grep -q "^client-connect" $config || echo "client-connect /usr/bin/docker-entrypoint.sh" >> $config
    grep -q "^client-disconnect" $config || echo "client-disconnect /usr/bin/docker-entrypoint.sh" >> $config
}

add_history() {
    #https://build.openvpn.net/man/openvpn-2.6/openvpn.8.html#environmental-variables
    set +e
    data="vip=$ifconfig_pool_remote_ip&rip=$trusted_ip&common_name=$common_name&username=$username&bytes_received=$bytes_received&bytes_sent=$bytes_sent&time_unix=$time_unix&time_duration=$time_duration"
    status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -d $data $ovpn_history_api)
    if [[ $? -ne 0 || $status -ne 200  ]]; then
        echo "[CLIENT-DISCONNECT] $0:$LINENO 保存历史记录出错，请检查！"
    fi
    set -e
}

set_ovip() {
    cc_file="$1"
    ip_file="$ovpn_data/.ovip"

    if [ -f "$ip_file" ]; then
        ipaddr=$(cat $ip_file)
        if [ -n "$ipaddr" ]; then
            echo "ifconfig-push $ipaddr $ifconfig_netmask" > $cc_file
            rm -rf $ip_file
        fi
    fi
}

client_disconnect() {
    add_history
}

client_connect() {
    set_ovip "$1"
}

################################################################################################

if [ "${1#-}" != "$1" ]; then
    /usr/sbin/openvpn "$@"
fi

case $1 in
    "genclient")
        if [ -z $2 ]; then
            echo "请输入生成客户端名称！"
            exit 1
        fi

        if [ -n "$6" ]; then
            echo -e "$6" > $OVPN_DATA/ccd/$2
        fi

        genclient "$2" "$3" "$4" "$5" "$7"
        exit 0
        ;;
    "auth")
        auth $2
        exit 0
        ;;
    "renewcert")
        renew_cert $2
        exit 0
        ;;
    "/usr/sbin/openvpn")
        if [ ! -f "$OVPN_DATA/server.conf" ]; then
            mkdir -p $OVPN_DATA/ccd

            init_pki
            init_config
        fi

        check_config
        run_server
        ;;
    "/usr/bin/supervisord")
        /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
        ;;
esac

case "$script_type" in
    client-connect)
        client_connect "$@"
        exit 0
        ;;
    client-disconnect)
        client_disconnect "$@"
        exit 0
        ;;
esac

exec "$@"
