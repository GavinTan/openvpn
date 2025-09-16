FROM alpine:3.22.1

RUN apk add --no-cache easy-rsa openvpn iptables iptables-legacy bash supervisor curl sqlite grep
RUN ln -s /usr/share/easy-rsa/easyrsa /usr/local/bin
#RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

COPY docker-entrypoint.sh /usr/bin
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf

RUN wget https://github.com/gavintan/openvpn/releases/latest/download/openvpn-web-$(uname -s)-$(uname -m) -O /usr/local/bin/openvpn-web
RUN wget https://github.com/gavintan/openvpn/releases/latest/download/openvpn-auth-$(uname -s)-$(uname -m) -O /usr/lib/openvpn/plugins/openvpn-auth
RUN chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth

VOLUME ["/data"]

ENV OVPN_DATA=/data
ENV OVPN_SUBNET=10.8.0.0/24
ENV OVPN_SUBNET6=fdaf:f178:e916:6dd0::/64
ENV OVPN_MAXCLIENTS=200
ENV OVPN_PROTO=udp
ENV OVPN_PORT=1194
ENV OVPN_IPV6=false
ENV OVPN_GATEWAY=false
ENV OVPN_MANAGEMENT=127.0.0.1:7505
ENV AUTH_API=http://127.0.0.1:8833/login
ENV OVPN_AUTH_API=http://127.0.0.1:8833/ovpn/login
ENV OVPN_HISTORY_API=http://127.0.0.1:8833/ovpn/history
ENV GIN_MODE=release
ENV ADMIN_USERNAME=admin
ENV ADMIN_PASSWORD=admin
ENV WEB_PORT=8833
ENV ENV_UPDATE_CONFIG=true
ENV OVPN_LDAP_AUTH=false
ENV LDAP_URL=ldap://example.org:389
ENV LDAP_BASE_DN=dc=example,dc=org
ENV LDAP_USER_ATTRIBUTE=uid
ENV LDAP_USER_GROUP_FILTER=false
ENV LDAP_USER_GROUP_DN=cn=vpn,ou=groups,dc=example,dc=org
ENV LDAP_USER_ATTR_IPADDR_NAME=ipaddr
ENV LDAP_BIND_USER_DN=cn=admin,dc=example,dc=org
ENV LDAP_BIND_PASSWORD=adminpassword
#fix https://github.com/GavinTan/openvpn/issues/27
ENV move_req_and_key=true

EXPOSE 1194/udp 8833

USER root

WORKDIR /data


ENTRYPOINT  ["docker-entrypoint.sh"]

CMD ["/usr/bin/supervisord"]
