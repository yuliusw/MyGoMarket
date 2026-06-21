package main

import (
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/services/iam/app"
	"github.com/yuliusw/RPA-market/services/iam/repository"
)

func RegisterService() {

	middleware.InitJWTAuth(repository.NewUserRepository(database.DB, database.RedisClient))
	app.InitGroup(repository.NewGroupRepository())
	app.InitRole(repository.NewRoleRepository())
	app.InitUser(repository.NewUserRepository(database.DB, database.RedisClient), database.GlobalMinio)
}
