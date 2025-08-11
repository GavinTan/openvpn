#!/bin/bash
set -e

init_env() {
    cat << EOF > $OVPN_DATA/pki/vars
EASYRSA_PKI=$OVPN_DATA/pki
EASYRSA_CA_EXPIRE=3650
EASYRSA_CERT_EXPIRE=365
EASYRSA_CRL_DAYS=3650
EASYRSA_ALGO=ec
EASYRSA_CURVE=prime256v1
EOF
    [ ! -f "$OVPN_DATA/.vars" ] && cat << EOF > $OVPN_DATA/.vars
SECRET_KEY=$(head /dev/urandom | tr -dc A-Za-z0-9 | fold -w 50 | head -n 1)
SERVER_NAME=server_$(head /dev/urandom | tr -dc A-Za-z0-9 | fold -w 16 | head -n 1)
SERVER_CN=ovpn_$(head /dev/urandom | tr -dc A-Za-z0-9 | fold -w 16 | head -n 1)
EOF
    source $OVPN_DATA/.vars
}

init_pki() {
    cd $OVPN_DATA && /usr/share/easy-rsa/easyrsa init-pki
    init_env
    /usr/share/easy-rsa/easyrsa --batch --req-cn="$SERVER_CN" build-ca nopass
    /usr/share/easy-rsa/easyrsa --batch build-server-full "$SERVER_NAME" nopass
    /usr/share/easy-rsa/easyrsa gen-crl
    /usr/sbin/openvpn --genkey secret $OVPN_DATA/pki/tc.key
}

init_config() {
    cat << EOF > $OVPN_DATA/server.conf
port $OVPN_PORT
proto $OVPN_PROTO
dev tun
persist-key
persist-tun
keepalive 10 120
topology subnet
$([[ "$OVPN_IPV6" == "true" ]] && echo -e "server $(getsubnet $OVPN_SUBNET)\nserver-ipv6 $OVPN_SUBNET6" || echo "server $(getsubnet $OVPN_SUBNET)")
$([[ "$OVPN_GATEWAY" == "true" ]] && echo -e 'push "dhcp-option DNS 8.8.8.8"\npush "dhcp-option DNS 8.8.4.4"\npush "redirect-gateway def1 ipv6 bypass-dhcp"' || echo -e '#push "dhcp-option DNS 8.8.8.8"\n#push "dhcp-option DNS 8.8.4.4"\n#push "redirect-gateway def1 ipv6 bypass-dhcp"')
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
keepalive 10 60
duplicate-cn
client-to-client
max-clients $OVPN_MAXCLIENTS
management ${OVPN_MANAGEMENT/:/ }
verb 2
$([[ "$OVPN_PROTO" =~ "udp" ]]  && echo "explicit-exit-notify 1")
setenv ovpn_data ${OVPN_DATA:-/data}
setenv auth_api ${AUTH_API:-http://127.0.0.1/login}
setenv ovpn_auth_api ${OVPN_AUTH_API:-http://127.0.0.1/ovpn/login}
setenv ovpn_history_api ${OVPN_HISTORY_API:-http://127.0.0.1:8833/ovpn/history}
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

    $ipt -t nat -C POSTROUTING -s $OVPN_SUBNET -j MASQUERADE > /dev/null 2>&1 || {
        $ipt -t nat -A POSTROUTING -s $OVPN_SUBNET -j MASQUERADE
    }

    if [ "$OVPN_IPV6" == "true" ]; then
        ${ipt/iptables/ip6tables} -t nat -C POSTROUTING -s $OVPN_SUBNET6 -j MASQUERADE > /dev/null 2>&1 || {
            ${ipt/iptables/ip6tables} -t nat -A POSTROUTING -s $OVPN_SUBNET6 -j MASQUERADE
        }
    fi

    /usr/sbin/openvpn $OVPN_DATA/server.conf
}

update_config() {
    source $OVPN_DATA/.vars

    config=$OVPN_DATA/server.conf
    auth_api=$(grep '^setenv auth_api' $config | cut -d' ' -f3)
    ovpn_auth_api=$(grep '^setenv ovpn_auth_api' $config | cut -d' ' -f3)
    ovpn_history_api=$(grep '^setenv ovpn_history_api' $config | cut -d' ' -f3)
    ovpn_data=$(grep '^setenv ovpn_data' $config | cut -d' ' -f3)
    ovpn_subnet=$(grep '^server' $config | cut -d' ' -f2,3)
    ovpn_subnet6=$(grep '^server-ipv6' $config | cut -d' ' -f2,3)
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

    if [ "$OVPN_IPV6" == "true" ]; then
        if [[ ! "$(grep '^proto' $config | cut -d' ' -f2)" =~ 6 ]]; then
            sed -i "s|^proto .*|proto ${OVPN_PROTO}6|" $config
        fi

        if [ "$ovpn_subnet6" != "$OVPN_SUBNET6" ]; then
            if [ -z "$ovpn_subnet6" ]; then
                echo "server-ipv6 $OVPN_SUBNET6" >> $config
            else
                sed -i "s|^server-ipv6 .*|server-ipv6 $OVPN_SUBNET6|" $config
            fi
        fi
    else
        sed -i "/^server-ipv6/d" $config
    fi

    if [ "$OVPN_GATEWAY" == "true" ]; then
        sed -i 's/^#\(push "dhcp-option DNS 8.8.8.8"\)/\1/' $config
        sed -i 's/^#\(push "dhcp-option DNS 8.8.4.4"\)/\1/' $config
        sed -i 's/^#\(push "redirect-gateway def1 ipv6 bypass-dhcp"\)/\1/' $config
    else
        sed -i 's/^push "dhcp-option DNS 8.8.8.8"/#&/' $config
        sed -i 's/^push "dhcp-option DNS 8.8.4.4"/#&/' $config
        sed -i 's/^push "redirect-gateway def1 ipv6 bypass-dhcp"/#&/' $config
    fi

    if [[ "$OVPN_PROTO" =~ "tcp" ]]; then
        sed -i "/^explicit-exit-notify/d" $config
    fi

    if [[ "$OVPN_PROTO" =~ "udp" ]]; then
        grep -q "^explicit-exit-notify" $config || echo "explicit-exit-notify 1" >> $config
    fi
}

renew_cert() {
    source $OVPN_DATA/.vars
    source $OVPN_DATA/pki/vars

    cd $OVPN_DATA/pki
    openssl x509 -in ca.crt -days $1 -out ca.crt -signkey private/ca.key
    /usr/share/easy-rsa/easyrsa --batch --days=$1 renew $SERVER_NAME
    /usr/share/easy-rsa/easyrsa --batch revoke-renewed $SERVER_NAME
    /usr/share/easy-rsa/easyrsa --batch gen-crl
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
    if [ ! -f "$EASYRSA_PKI/private/$1.key" ]; then
        /usr/share/easy-rsa/easyrsa --batch build-client-full $1 nopass > /dev/null
    fi
    mkdir -p $OVPN_DATA/clients
    cat << EOF > $OVPN_DATA/clients/$1.ovpn
client
proto $([[ "$OVPN_IPV6" == "true" ]] && [[ ! "$OVPN_PROTO" =~ 6 ]] && echo "${OVPN_PROTO}6" || echo $OVPN_PROTO)
remote ${2:-$([[ "$OVPN_IPV6" == "true" ]] && ip -6 route get 2001:4860:4860::8888 | grep -oP 'src \K\S+' || ip -4 route get 8.8.8.8 | grep -oP 'src \K\S+')} $OVPN_PORT
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
$([[ "$OVPN_PROTO" =~ "udp" ]] && echo "explicit-exit-notify")

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

check_config() {
    config=$OVPN_DATA/server.conf
    grep -q "^client-connect" $config || echo "client-connect /usr/bin/docker-entrypoint.sh" >> $config
    grep -q "^client-disconnect" $config || echo "client-disconnect /usr/bin/docker-entrypoint.sh" >> $config
}

add_history() {
    #https://build.openvpn.net/man/openvpn-2.6/openvpn.8.html#environmental-variables
    data="vip=$ifconfig_pool_remote_ip&rip=$trusted_ip&common_name=$common_name&username=$username&bytes_received=$bytes_received&bytes_sent=$bytes_sent&time_unix=$time_unix&time_duration=$time_duration"
    status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -d $data $ovpn_history_api)

    [ $status -ne 200 ] && echo "[CLIENT-DISCONNECT] $0:$LINENO 保存历史记录出错，请检查！" || true
}

client_disconnect() {
    set +e
    add_history
    [ $? -ne 0 ] && echo "[CLIENT-DISCONNECT] $0:$LINENO 保存历史记录出错，请检查！"
    set -e
}

client_connect() {
    #set static ip
    cc_file="$1"
    sql="SELECT ip_addr FROM user WHERE username='$username'"
    ipaddr=$(sqlite3 $ovpn_data/ovpn.db "$sql")
    [ -n "$ipaddr" ] && echo "ifconfig-push $ipaddr $ifconfig_netmask" > $cc_file || true
}

next_clientaddr() {
    # 获取子网基础 IP
    subnet_ip=$(echo -e $OVPN_SUBNET | awk -F/ '{print $1}')
    subnet_as_int=$(echo $subnet_ip | awk -F. '{print ($1*256^3)+($2*256^2)+($3*256)+$4}')
    
    # 服务器 IP 是子网 IP + 1
    server_as_int=$((subnet_as_int + 1))
    
    # 获取已分配的 IP 列表
    allocated_ips=""
    if [ -d "$OVPN_DATA/ccd" ]; then
        # 提取所有 ccd 文件中的 IP 地址
        allocated_ips=$(grep -r "ifconfig-push" $OVPN_DATA/ccd 2>/dev/null | awk '{print $2}')
    fi
    
    # 获取子网掩码中的网络位数
    prefix=$(echo $OVPN_SUBNET | cut -d'/' -f2)
    
    # 计算可用主机数量上限（减去网络地址、广播地址和服务器地址）
    max_hosts=$((2**(32-prefix) - 3))
    
    # 从 2 开始尝试（服务器是 1）
    for i in $(seq 1 $max_hosts); do
        # 计算当前尝试的 IP 地址
        current_as_int=$((server_as_int + i))
        
        # 转换为 IP 地址格式
        current_ip=$(echo $current_as_int | awk '{printf("%d.%d.%d.%d", ($1/256^3)%256, ($1/256^2)%256, ($1/256)%256, $1%256)}')
        
        # 检查 IP 是否已分配
        if ! echo "$allocated_ips" | grep -q "$current_ip"; then
            # 找到未分配的 IP
            echo $current_ip
            return 0
        fi
    done
    
    # 如果所有 IP 都已分配，返回错误
    echo "Error: No available IP addresses in subnet $OVPN_SUBNET" >&2
    return 1
}

get_ccd() {
    netmask=$(getsubnet $OVPN_SUBNET | awk -F' ' '{print $2}')
    client_addr=$(next_clientaddr)
    ccd="ifconfig-push $client_addr $netmask"
    echo $ccd
}

################################################################################################

if [ "$1" == "--init" ]; then
    mkdir -p $OVPN_DATA/ccd

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

        # if [ -n "$5" ]; then
            # mkdir -p $OVPN_DATA/ccd
            # echo -e "$5" > $OVPN_DATA/ccd/$2
        # fi
        ccd=$(get_ccd)
        mkdir -p $OVPN_DATA/ccd
        echo -e "$ccd" > $OVPN_DATA/ccd/$2

        genclient $2 $3 "$4"
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
        [[ "$ENV_UPDATE_CONFIG" == "true" ]] && update_config
        check_config
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
