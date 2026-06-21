package main

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/services/iam"
)

func main() {
	// 1. 初始化数据库
	database.InitGORM()

	// 2. 设置路由
	r := gin.Default()
	iam.RegisterHandlers(r)

	// 3. 启动
	r.Run(":8080")
}
