package utils

import (
	"io"
	"os"
	"path/filepath"
)

// WriteAt 在指定位置写入数据，用于分片合并或随机写
func WriteAt(path string, data []byte, offset int64) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteAt(data, offset)
	return err
}

// ReadChunk 读取文件的特定片段
func ReadChunk(path string, offset int64, size int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content := make([]byte, size)
	_, err = file.ReadAt(content, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return content, nil
}

// EnsureDir 确保目录存在
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, os.ModePerm)
}

// GetFileSize 获取文件大小
func GetFileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
