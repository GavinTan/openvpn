FROM alpine:3.22.2

RUN apk add --no-cache openssl openvpn iptables iptables-legacy bash supervisor curl sqlite grep mailcap coreutils jq
#RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

COPY docker-entrypoint.sh /usr/bin
COPY openvpn-auth /usr/lib/openvpn/plugins/openvpn-auth
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf

RUN wget https://github.com/OpenVPN/easy-rsa/releases/download/v3.2.4/EasyRSA-3.2.4.tgz && tar -ozxf EasyRSA-3.2.4.tgz && mv EasyRSA-3.2.4 /usr/share/easy-rsa && rm -rf EasyRSA-3.2.4.tgz
RUN wget https://github.com/gavintan/openvpn/releases/latest/download/openvpn-web-$(uname -s)-$(uname -m) -O /usr/local/bin/openvpn-web
RUN chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth
RUN ln -s /usr/share/easy-rsa/easyrsa /usr/local/bin

VOLUME ["/data"]

ENV GIN_MODE=release
ENV OVPN_DATA=/data

EXPOSE 1194/udp 8833

USER root

WORKDIR /data


ENTRYPOINT  ["docker-entrypoint.sh"]

CMD ["/usr/bin/supervisord"]
