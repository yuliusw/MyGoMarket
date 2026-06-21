package database

import (
	"context"
	"log"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/yuliusw/RPA-market/common/config"
)

var GlobalMinio *MinioClient

type MinioClient struct {
	Client     *minio.Client
	BucketName string
}

// InitMinio 初始化 MinIO 连接
func InitMinio() {
	// 1. 从全局配置对象获取配置 (参考你的 RedisConfig 结构)
	minioConf := config.AppConfig.MinIO

	// 2. 初始化客户端
	client, err := minio.New(minioConf.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioConf.AccessKey, minioConf.SecretKey, ""),
		Secure: minioConf.UseSSL,
	})
	if err != nil {
		log.Fatalf("Failed to create MinIO client: %v", err)
	}

	// 3. 赋值给全局变量
	GlobalMinio = &MinioClient{
		Client:     client,
		BucketName: minioConf.BucketName,
	}
	log.Println("MinIO start init...")
	// 4. 使用带超时的 context 检查连接及存储桶状态
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, minioConf.BucketName)
	if err != nil {
		log.Fatalf("Failed to connect to MinIO or check bucket: %v", err)
	}

	// 5. 自动创建存储桶逻辑
	if !exists {
		err = client.MakeBucket(ctx, minioConf.BucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Fatalf("Failed to create bucket [%s]: %v", minioConf.BucketName, err)
		}
		log.Printf("MinIO bucket [%s] created successfully", minioConf.BucketName)
	}

	log.Println("MinIO connected successfully")
}

// UploadFile 上传本地文件
func (m *MinioClient) UploadFile(ctx context.Context, objectName, filePath, contentType string) error {
	_, err := m.UploadFileInfo(ctx, objectName, filePath, contentType)
	return err
}

func (m *MinioClient) UploadFileInfo(ctx context.Context, objectName, filePath, contentType string) (minio.UploadInfo, error) {
	info, err := m.Client.FPutObject(ctx, m.BucketName, objectName, filePath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return minio.UploadInfo{}, err
	}
	log.Printf("文件上传成功: %s, Size: %d", objectName, info.Size)
	return info, nil
}

// GetFileUrl 获取文件的临时访问链接（预签名 URL）
// expires: 链接有效期，例如 time.Hour * 2
func (m *MinioClient) GetFileUrl(ctx context.Context, objectName string, expires time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := m.Client.PresignedGetObject(ctx, m.BucketName, objectName, expires, reqParams)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

// RemoveFile 删除文件
func (m *MinioClient) RemoveFile(ctx context.Context, objectName string) error {
	return m.Client.RemoveObject(ctx, m.BucketName, objectName, minio.RemoveObjectOptions{})
}

// DownloadFile 下载文件到本地
func (m *MinioClient) DownloadFile(ctx context.Context, objectName, filePath string) error {
	return m.Client.FGetObject(ctx, m.BucketName, objectName, filePath, minio.GetObjectOptions{})
}
