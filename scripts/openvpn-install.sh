#!/bin/bash
set -e

#安装数据目录
OVPN_DATA="/data/openvpn"

OPENVPN_VERSION="2.6.17"

SUPERVISOR_CONF="/etc/supervisord.conf"
SUPERVISOR_START="systemctl restart supervisor"

BASE_DIR=`pwd`
TMP_DIR="/tmp/ovpn"
mkdir -p $TMP_DIR

check_env() {
    . /etc/os-release

    case "$ID" in
        ubuntu|debian)
            apt install -y wget gzip tar g++ make iproute2 pkg-config libnl-genl-3-dev libcap-ng-dev libssl-dev liblz4-dev liblzo2-dev libpam0g-dev libcmocka-dev libpkcs11-helper1-dev
            apt install -y openssl iptables supervisor curl sqlite3 grep mailcap coreutils jq bash

            SUPERVISOR_CONF="/etc/supervisor/supervisord.conf"
            SUPERVISOR_START="systemctl restart supervisor"

            mkdir -p /etc/systemd/system/supervisor.service.d
            cat << EOF > /etc/systemd/system/supervisor.service.d/override.conf
[Service]
EnvironmentFile=-/etc/environment
EOF
            systemctl daemon-reload
            ;;
        centos|rhel|fedora|rocky|almalinux|ol)
            dnf install -y dnf-plugins-core epel-release
            dnf config-manager --set-enabled crb
            dnf install -y wget gzip bzip2 tar gcc-c++ make iproute libnl3-devel libcap-ng-devel openssl-devel lz4-devel lzo-devel pam-devel libcmocka-devel
            dnf install -y openssl iptables supervisor curl sqlite grep mailcap coreutils jq bash
            
            cd $TMP_DIR
            wget https://github.com/OpenSC/pkcs11-helper/releases/download/pkcs11-helper-1.31.0/pkcs11-helper-1.31.0.tar.bz2
            tar jxf pkcs11-helper-1.31.0.tar.bz2
            cd pkcs11-helper-1.31.0
            ./configure
            make && make install

            export PKG_CONFIG_PATH=/usr/local/lib/pkgconfig:$PKG_CONFIG_PATH
            SUPERVISOR_CONF="/etc/supervisord.conf"
            SUPERVISOR_START="systemctl restart supervisord"

            mkdir -p /etc/systemd/system/supervisord.service.d
            cat << EOF > /etc/systemd/system/supervisord.service.d/override.conf
[Service]
EnvironmentFile=-/etc/environment
EOF
            systemctl daemon-reload
            ;;
        alpine)
            apk add wget gzip bzip2 tar g++ make iproute2 pkgconf linux-headers libnl3-dev libcap-ng-dev openssl-dev lz4-dev lzo-dev linux-pam-dev cmocka-dev
            apk add openssl iptables supervisor curl sqlite grep mailcap coreutils jq bash

            cd $TMP_DIR
            wget https://github.com/OpenSC/pkcs11-helper/releases/download/pkcs11-helper-1.31.0/pkcs11-helper-1.31.0.tar.bz2
            tar jxf pkcs11-helper-1.31.0.tar.bz2
            cd pkcs11-helper-1.31.0
            ./configure
            make && make install

            export PKG_CONFIG_PATH=/usr/local/lib/pkgconfig:$PKG_CONFIG_PATH
            SUPERVISOR_CONF="/etc/supervisord.conf"
            SUPERVISOR_START="rc-service supervisord restart"

            echo "export OVPN_DATA=$OVPN_DATA" > /etc/profile.d/openvpn.sh
            echo "export OVPN_DATA=$OVPN_DATA" >> /etc/conf.d/supervisord
            ;;
        *)
            echo "不支持的系统！"
            exit 1
            ;;
    esac
}



install_openvpn() {
    cd $TMP_DIR
    wget https://github.com/OpenVPN/openvpn/releases/download/v$OPENVPN_VERSION/openvpn-$OPENVPN_VERSION.tar.gz
    tar -zxf openvpn-$OPENVPN_VERSION.tar.gz
    cd openvpn-$OPENVPN_VERSION

	./configure \
		--prefix=/usr \
		--mandir=/usr/share/man \
		--sysconfdir=/etc/openvpn \
		--enable-dco \
		--enable-pkcs11 \
		--enable-x509-alt-username
	make
    make install
}

install_easyrsa() {
    cd $TMP_DIR
    wget https://github.com/OpenVPN/easy-rsa/releases/download/v3.2.5/EasyRSA-3.2.5.tgz
    tar -ozxf EasyRSA-3.2.5.tgz
    mv EasyRSA-3.2.5 /usr/share/easy-rsa
}

install_openvpnweb() {
    wget https://github.com/gavintan/openvpn/releases/latest/download/openvpn-web-$(uname -s)-$(uname -m) -O /usr/local/bin/openvpn-web 
}


install(){
    install_openvpn
    install_easyrsa
    install_openvpnweb
    
    cd $BASE_DIR
    cp ../build/docker-entrypoint.sh /usr/bin
    cp ../build/openvpn-auth /usr/lib/openvpn/plugins/openvpn-auth
    cp ../build/supervisord.conf $SUPERVISOR_CONF

    chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth
    ln -s /usr/share/easy-rsa/easyrsa /usr/local/bin

    mkdir -p $OVPN_DATA/logs
    sed -i "s|^nodaemon=.*$|nodaemon=false|" $SUPERVISOR_CONF
    sed -i "s|^stdout_logfile=.*$|stdout_logfile=$OVPN_DATA/logs/openvpn.log|" $SUPERVISOR_CONF
    sed -i "s|^stdout_logfile_maxbytes=.*$|stdout_logfile_maxbytes=30MB|" $SUPERVISOR_CONF

    echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf && sysctl -p >> /dev/null
    echo "OVPN_DATA=$OVPN_DATA" >> /etc/environment
}


main(){
    check_env
    install
    $SUPERVISOR_START
    rm -rf $TMP_DIR

    echo "安装完成！http://<ip>:8833 访问openvpn-web，用户名：admin，密码：admin"
}


main