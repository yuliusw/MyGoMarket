package utils

// AllowDomainSuffixes 定义允许的域名后缀
// 这样可以支持所有以 .baidu.com 结尾的子域名
var AllowDomainSuffixes = []string{
	".baidu.com",
	".yuliusw.com",    // 你的线上项目域名
	"localhost:5173",  // 本地 Vite 前端开发 (跨域 Origin 通常是这个)
	"127.0.0.1:5173",  // 兼容本地通过 IP 访问前端的情况
	"localhost:12660", // 本地后端服务端口 (防止某些同源策略的严格校验)
	"127.0.0.1:12660",
}
