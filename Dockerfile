# 使用官方 Golang 1.21 镜像作为基础
FROM golang:1.21

# 安装 SSH 服务
RUN apt-get update && \
    apt-get install -y openssh-server && \
    mkdir /var/run/sshd && \
    echo 'root:123456' | chpasswd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# 设置工作目录
WORKDIR /app/epoll

# 将当前目录的所有文件复制到容器中
COPY . .

# 暴露端口
EXPOSE 8080
EXPOSE 22

# 启动 SSH 服务和保持容器运行的命令
CMD ["sh", "-c", "service ssh start && /bin/bash"]