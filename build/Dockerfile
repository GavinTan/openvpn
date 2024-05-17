FROM alpine:3.19.1

RUN apk add --no-cache easy-rsa openvpn iptables bash supervisor
RUN ln -s /usr/share/easy-rsa/easyrsa /usr/local/bin
#RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

COPY docker-entrypoint.sh /usr/bin
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf

RUN wget https://github.com/gavintan/openvpn-web/releases/latest/download/openvpn-web-$(uname -s)-$(uname -m) -O /usr/local/bin/openvpn-web
RUN wget https://github.com/gavintan/openvpn-auth/releases/latest/download/openvpn-auth-$(uname -s)-$(uname -m) -O /usr/lib/openvpn/plugins/openvpn-auth
RUN chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth

VOLUME ["/data"]

ENV OVPN_DATA=/data
ENV OVPN_SUBNET=10.8.0.0/24
ENV OVPN_MAXCLIENTS=200
ENV OVPN_PROTO=udp
ENV OVPN_PORT=1194
ENV OVPN_MANAGE_PORT=7505
ENV AUTH_API=http://127.0.0.1/login
ENV OVPN_AUTH_API=http://127.0.0.1/ovpn/login
ENV GIN_MODE=release
ENV ADMIN_USERNAME=admin
ENV ADMIN_PASSWORD=admin

EXPOSE 1194/udp 80

USER root

WORKDIR /data


ENTRYPOINT  ["docker-entrypoint.sh"]

CMD ["/usr/bin/supervisord"]
