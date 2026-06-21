package services

import (
	"fmt"
	"io"
	"os"

	"github.com/yuliusw/RPA-market/common/utils"
)

type FileTask struct {
	FileID    string // 文件的唯一标识
	FilePath  string // 最终存储路径
	ChunkDir  string // 分片暂存目录
	TotalSize int64
	ChunkSize int64
}

// NewFileTask 初始化任务
func NewFileTask(path string, chunkSize int64) *FileTask {
	// 实际开发中 FileID 应由前端传 MD5，此处简写
	return &FileTask{
		FilePath:  path,
		ChunkDir:  path + "_chunks",
		ChunkSize: chunkSize,
	}
}

// UploadChunk 上传单个分片
func (t *FileTask) UploadChunk(chunkData []byte, index int) error {
	utils.EnsureDir(t.ChunkDir)
	chunkPath := fmt.Sprintf("%s/%d", t.ChunkDir, index)

	// 断点续传检查：如果分片已存在且大小正确，则跳过
	if fi, err := os.Stat(chunkPath); err == nil && fi.Size() == int64(len(chunkData)) {
		return nil
	}

	return os.WriteFile(chunkPath, chunkData, 0644)
}

// MergeChunks 合并所有分片
func (t *FileTask) MergeChunks(totalChunks int) error {
	fullFile, err := os.Create(t.FilePath)
	if err != nil {
		return err
	}
	defer fullFile.Close()

	for i := 0; i < totalChunks; i++ {
		chunkPath := fmt.Sprintf("%s/%d", t.ChunkDir, i)
		chunkData, err := os.ReadFile(chunkPath)
		if err != nil {
			return err
		}
		fullFile.Write(chunkData)
		os.Remove(chunkPath) // 合并后删除分片
	}
	os.RemoveAll(t.ChunkDir)
	return nil
}

// DownloadRange 模拟处理下载请求
// start: 起始字节, end: 结束字节
func (t *FileTask) DownloadRange(start, end int64) ([]byte, error) {
	file, err := os.Open(t.FilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	size := end - start + 1
	data := make([]byte, size)

	_, err = file.Seek(start, io.SeekStart)
	if err != nil {
		return nil, err
	}

	_, err = io.ReadFull(file, data)
	return data, err
}
