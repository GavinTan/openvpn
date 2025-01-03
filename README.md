# openvpn

**docker 版[openvpn](https://hub.docker.com/r/yyxx/openvpn)，支持 web 管理。**

openvpn 安全与加密相关配置参考于[openvpn-install](https://github.com/angristan/openvpn-install?tab=readme-ov-file#security-and-encryption)的Security and Encryption部分。

![image-20240529110841439](https://raw.githubusercontent.com/GavinTan/files/master/picgo/image-20240529110841439.png)

![20220930173030](https://raw.githubusercontent.com/GavinTan/files/master/picgo/20220930173030.png)

![20220930173103](https://raw.githubusercontent.com/GavinTan/files/master/picgo/20220930173103.png)

![image-20241015170847764](https://raw.githubusercontent.com/GavinTan/files/master/picgo/image-20241015170847764.png)

> 提示：
>
> 1. 登录账号密码默认`admin:admin`可通过环境变量修改
> 2. web->管理->客户端里生成下载客户端配置文件
> 3. web->管理->VPN 账号里管理添加账号，默认启用账号验证可在 VPN 账号里开启或关闭。
>
> 
>
> 注意：
>
> 1. 默认禁用vpn网关，如果需要客户端所有流量都走 openvpn 请使用环境变量`OVPN_GATEWAY=true`。
> 2. 在创建客户端后关闭账号验证客户端的配置文件存在auth-user-pass参数客户端会依旧弹出登录，登录信息可以随便输入不会做验证，若有弹窗困扰的建议手动编辑客户端配置文件注释掉参数或重新生成客户端配置文件。

## Quick Start

初始化生成证书及配置文件

```shell
docker run -v $(pwd)/data:/data --rm yyxx/openvpn --init
```

运行 openvpn

```shell
docker run -d \
  --cap-add=NET_ADMIN \
  -p 1194:1194/udp \
  -p 8833:8833 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=admin \
  -v $(pwd)/data:/data \
  yyxx/openvpn
```

### compose

- 安装 docker-compose

  ```bash
  curl -SL https://github.com/docker/compose/releases/download/v2.30.3/docker-compose-linux-x86_64 -o /usr/local/bin/docker-compose
  chmod +x /usr/local/bin/docker-compose
  ```

- 创建 docker-compose.yml

  ```yaml
  services:
    openvpn:
      image: yyxx/openvpn
      cap_add:
        - NET_ADMIN
      ports:
        - "1194:1194/udp"
        - "8833:8833"
      environment:
        - ADMIN_USERNAME=admin
        - ADMIN_PASSWORD=admin
      volumes:
        - ./data:/data
        - /etc/localtime:/etc/localtime:ro
  ```
  
- 初始化生成证书及配置文件

  ```bash
  docker-compose run --rm openvpn --init
  ```

- 运行 openvpn

  ```bash
  docker-compose up -d
  ```



## IPV6

>注意：
>
>1. 启用ipv6后客户端跟服务器的proto需要都指定udp6/tcp6
>2. docker的网络需要启用ipv6支持
>3. 使用openvpn-connect客户端的需要使用3.4.1以后的版本

```bash
services:
  openvpn:
    image: yyxx/openvpn
    cap_add:
      - NET_ADMIN
    ports:
      - "1194:1194/udp"
      - "8833:8833"
    environment:
      - ADMIN_USERNAME=admin
      - ADMIN_PASSWORD=admin
      - OVPN_IPV6=true
    volumes:
      - ./data:/data
      - /etc/localtime:/etc/localtime:ro
    sysctls:
      - net.ipv6.conf.default.disable_ipv6=0
      - net.ipv6.conf.all.forwarding=1

networks:
  default:
    enable_ipv6: true
```



## 环境变量参数

|       环境变量        |             说明             |               默认值               |
| :-------------------: | :--------------------------: | :--------------------------------: |
|     **OVPN_DATA**     |         数据存放目录         |               /data                |
|    **OVPN_SUBNET**    |           vpn子网            |            10.8.0.0/24             |
|   **OVPN_SUBNET6**    |         vpn ipv6子网         |      fdaf:f178:e916:6dd0::/64      |
|    **OVPN_PROTO**     |      协议 tcp(6)/udp(6)      |                udp                 |
|     **OVPN_PORT**     |         vpn连接端口          |                1194                |
|  **OVPN_MAXCLIENTS**  |     vpn最大客户端连接数      |                200                 |
|  **OVPN_MANAGEMENT**  |   openvpn管理接口监听地址    |           127.0.0.1:7505           |
|   **OVPN_AUTH_API**   |        vpn账号认证api        |  http://127.0.0.1:8833/ovpn/login  |
| **OVPN_HISTORY_API**  |        vpn历史记录api        | http://127.0.0.1:8833/ovpn/history |
|     **OVPN_IPV6**     |           启用ipv6           |               false                |
|   **OVPN_GATEWAY**    |   启用vpn网关所有流量走vpn   |               false                |
|     **WEB_PORT**      |           web端口            |                8833                |
|     **AUTH_API**      |        web登录认证api        |    http://127.0.0.1:8833/login     |
|  **ADMIN_USERNAME**   |         web登录账号          |               admin                |
|  **ADMIN_PASSWORD**   |         web登录密码          |               admin                |
| **ENV_UPDATE_CONFIG** | 启用环境变量自动更新配置文件 |                true                |
