# openvpn

**docker版[openvpn](https://hub.docker.com/r/yyxx/openvpn)，支持web管理。**

openvpn安全与加密相关配置参考于[openvpn-install](https://github.com/angristan/openvpn-install)。



> 提示：web->管理->客户端里生成下载客户端配置文件，web->管理->VPN账号里管理添加账号，默认启用账号验证可在VPN账号里开启或关闭。
> 
> 注意：默认生成的server.conf配置文件里push "redirect-gateway def1 bypass-dhcp"是禁用的，如果需要客户端所有流量都走openvpn请把配置文件里push前面注释去掉。

![20220930173030](https://raw.githubusercontent.com/GavinTan/files/master/picgo/20220930173030.png)



![20220930173103](https://raw.githubusercontent.com/GavinTan/files/master/picgo/20220930173103.png)



## Quick Start

- 安装docker-compose

  ~~~bash
  curl -SL https://github.com/docker/compose/releases/download/v2.11.2/docker-compose-linux-x86_64 -o /usr/local/bin/docker-compose
  chmod +x /usr/local/bin/docker-compose
  ~~~
  
- 创建docker-compose.yml

  ~~~yaml
  version: "3.9"
  services:
    openvpn:
      image: yyxx/openvpn
      cap_add:
        - NET_ADMIN
      ports:
        - "1194:1194/udp"
        - "8833:80"
      volumes:
        - ./data:/data
        - /etc/localtime:/etc/localtime:ro
  ~~~
  
- 初始化生成证书配置文件

  ~~~bash 
  docker-compose run --rm openvpn --init
  ~~~

- 运行openvpn

  ~~~bash
  docker-compose up -d
  ~~~

  

