# openvpn

**docker 版[openvpn](https://hub.docker.com/r/yyxx/openvpn)，支持 web 管理。**

openvpn 安全与加密相关配置参考于[openvpn-install](https://github.com/angristan/openvpn-install)的Security and Encryption部分。

![image-20240529110841439](https://raw.githubusercontent.com/GavinTan/files/master/picgo/image-20240529110841439.png)

![20220930173030](https://raw.githubusercontent.com/GavinTan/files/master/picgo/20220930173030.png)

![20220930173103](https://raw.githubusercontent.com/GavinTan/files/master/picgo/20220930173103.png)

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
> 1. 默认生成的 server.conf 配置文件里 push "redirect-gateway def1 bypass-dhcp"是禁用的，如果需要客户端所有流量都走 openvpn 请把配置文件里 push 前面注释去掉然后docker-compose restart重启容器。
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
  curl -SL https://github.com/docker/compose/releases/download/v2.11.2/docker-compose-linux-x86_64 -o /usr/local/bin/docker-compose
  chmod +x /usr/local/bin/docker-compose
  ```

- 创建 docker-compose.yml

  ```yaml
  version: "3.9"
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

## 环境变量参数

- `OVPN_DATA`：数据目录
- `OVPN_SUBNET`：vpn子网
- `OVPN_PROTO`：协议 tcp/udp
- `OVPN_PORT`：端口
- `OVPN_MANAGEMENT`：openvpn管理接口监听地址
- `AUTH_API`：web登录认证api
- `OVPN_AUTH_API`：vpn账号认证api
- `WEB_PORT`：web端口
- `ADMIN_USERNAME`：web登录账号
- `ADMIN_PASSWORD`：web登录密码
