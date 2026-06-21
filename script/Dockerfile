# 使用轻量级的 alpine 镜像作为运行底座
FROM alpine:3.22.2

# 安装必要的库（Go 程序通常需要 cgo 相关运行库支持）
RUN apk add --no-cache ca-certificates libc6-compat tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone
# 设置工作目录
WORKDIR /app

# 将你的二进制文件拷贝进镜像
COPY main .
COPY config.yaml .
COPY RBAC.model .
# 确保有执行权限
RUN chmod +x ./main

# 暴露你的应用端口（假设是 8000）
EXPOSE 12660

# 启动程序
CMD ["./main"]