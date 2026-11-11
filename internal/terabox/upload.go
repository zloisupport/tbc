package terabox

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"tbc/internal/util"
	"time"

	"github.com/tidwall/gjson"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type UploadOptions struct {
	UploadChunkSize int64
	SplitThreshold  int64
	MaxConcurrency  int64
}

type filePiece struct {
	Hash      string
	Seek      int64
	PieceSize int64
}

type preCreateResult struct {
	FilePieces []*filePiece
	BlockList  string
	UploadId   string
	FileSize   int64
}

type uploadInfo struct {
	Index         int
	LocalFilePath string
	TargetPath    string
	FilePiece     *filePiece
	UploadId      string
	SenderChan    chan<- uploadResult
}

type uploadResult struct {
	Index    int
	UploadId string
	MD5      string
	Error    error
}

func (c *Client) preCreate(localPath, destination string, opts *UploadOptions) (*preCreateResult, error) {
	// obtain file info
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("error getting file info: %v", err)
	}
	fileSize := fileInfo.Size()
	filename := filepath.Base(localPath)

	remotePath := filepath.Join("/", destination, filename)

	var numChunks int64 = 1
	var filePieces []*filePiece
	var blockList []string

	// if file is large enough, split it
	if fileSize >= opts.SplitThreshold {
		numChunks = (fileSize + opts.UploadChunkSize - 1) / opts.UploadChunkSize
	}
	filePieces = make([]*filePiece, numChunks)
	blockList = make([]string, numChunks)

	for i := int64(0); i < numChunks; i++ {
		hashByte := md5.Sum(fmt.Appendf(nil, "%s_%03d", localPath, i))
		hashStr := hex.EncodeToString(hashByte[:])
		chunk := int64(math.Ceil(float64(fileSize) / float64(numChunks)))
		seek := chunk * i
		size := int64(math.Min(float64(fileSize-seek), float64(chunk)))

		filePieces[i] = &filePiece{
			Hash:      hashStr,
			Seek:      seek,
			PieceSize: size,
		}
		blockList[i] = hashStr
	}

	blockListJSON, err := json.Marshal(blockList)
	if err != nil {
		return nil, fmt.Errorf("error marshaling block list: %v", err)
	}
	blockListJSONstr := string(blockListJSON)

	body, err := util.GetResponse(c.client.R().
		SetQueryParams(map[string]string{
			"app_id":  appId,
			"jsToken": c.jsToken,
		}).
		SetFormData(map[string]string{
			"path":        remotePath,
			"autoinit":    "1",
			"target_path": "/", // current working directory
			"block_list":  blockListJSONstr,
			"size":        fmt.Sprintf("%d", fileSize),
		}).
		Post("/api/precreate"))

	if err != nil {
		return nil, fmt.Errorf("failed to get precreate response: %w", err)
	}
	if errno := gjson.GetBytes(body, "errno").Int(); errno != 0 {
		return nil, fmt.Errorf("precreate failed with errno: %d", errno)
	}

	uploadId := gjson.GetBytes(body, "uploadid").String()
	if uploadId == "" {
		return nil, fmt.Errorf("Error getting uploadId: %v", err)
	}

	return &preCreateResult{
		FilePieces: filePieces,
		BlockList:  blockListJSONstr,
		UploadId:   uploadId,
		FileSize:   fileSize,
	}, nil
}

func calcContentLength(fileSize int64, fileName string) int64 {
	var buf bytes.Buffer
	multipartWriter := multipart.NewWriter(&buf)
	multipartWriter.CreateFormFile("file", fileName)
	multipartWriter.Close()
	return int64(buf.Len()) + fileSize
}

func (c *Client) uploadChunk(ctx context.Context, pb *mpb.Progress, uploadInfo *uploadInfo) {
	resultChan := uploadInfo.SenderChan

	file, err := os.Open(uploadInfo.LocalFilePath)
	if err != nil {
		resultChan <- uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("failed to open file: %w", err)}
		return
	}
	defer file.Close()

	uploadEndpoint := strings.Replace(c.baseURL, "://www.", "://c-jp.", 1)
	if !strings.Contains(uploadEndpoint, "://c-jp.") {
		uploadEndpoint = "https://c-jp.terabox.com"
	}
	uploadEndpoint += "/rest/2.0/pcs/superfile2"
	fileName := filepath.Base(uploadInfo.LocalFilePath)

	// prepare pipe
	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)
	hashReader := io.NewSectionReader(file, uploadInfo.FilePiece.Seek, uploadInfo.FilePiece.PieceSize)
	calculatedHashChan := make(chan string, 1)

	// prepare progress bar
	bar, _ := pb.Add(uploadInfo.FilePiece.PieceSize,
		mpb.BarStyle().Lbound("▕").Filler("█").Tip("▌").Padding(" ").Rbound("▏").Build(),
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(fileName+fmt.Sprintf(".%03d: ", uploadInfo.Index), decor.WCSyncSpaceR),
			decor.OnCompleteOrOnAbort(decor.NewPercentage("%.1f", decor.WCSyncSpace), "100% "),
		),
		mpb.AppendDecorators(
			decor.OnCompleteOrOnAbort(decor.CountersKibiByte("%.2f / %.2f", decor.WCSyncSpace), "Done"),
			decor.OnCompleteOrOnAbort(decor.AverageSpeed(decor.SizeB1024(0), "% .2f", decor.WCSyncSpace), ""),
		),
	)

	fileReader := io.NewSectionReader(file, uploadInfo.FilePiece.Seek, uploadInfo.FilePiece.PieceSize)
	proxyReader := bar.ProxyReader(fileReader)
	defer proxyReader.Close()

	// calculate md5 hash
	go func(reader io.Reader, resultChan chan<- string) {
		defer close(resultChan)
		hash := md5.New()
		if _, err := io.Copy(hash, reader); err != nil {
			fmt.Println("Error calculating hash:", err.Error())
			resultChan <- ""
			return
		}
		resultChan <- hex.EncodeToString(hash.Sum(nil))
	}(hashReader, calculatedHashChan)

	// multipart body writer
	go func() {
		defer pipeWriter.Close()
		defer multipartWriter.Close()

		fileWriter, err := multipartWriter.CreateFormFile("file", "blob")
		if err != nil {
			fmt.Println("Error creating part:", err.Error())
			return
		}

		_, err = io.Copy(fileWriter, proxyReader)
		if err != nil {
			fmt.Println("Error copying part:", err.Error())
			return
		}
	}()

	u, err := url.Parse(uploadEndpoint)
	if err != nil {
		resultChan <- uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error parsing url: %w", err)}
		return
	}

	q := u.Query()
	q.Set("method", "upload")
	q.Set("type", "tmpfile")
	q.Set("app_id", appId)
	q.Set("path", fmt.Sprintf("%s/%s", strings.TrimSuffix(uploadInfo.TargetPath, "/"), fileName))
	q.Set("uploadid", uploadInfo.UploadId)
	q.Set("partseq", fmt.Sprintf("%d", uploadInfo.Index))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("POST", u.String(), pipeReader)
	if err != nil {
		resultChan <- uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error creating request: %w", err)}
		return
	}

	req = req.WithContext(ctx)

	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", c.baseURL)
	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}
	req.ContentLength = calcContentLength(uploadInfo.FilePiece.PieceSize, "blob")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		resultChan <- uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error uploading part: %w", err)}
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		resultChan <- uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error reading response: %w", err)}
		return
	}
	uploadedHash := gjson.GetBytes(body, "md5").String()
	calculatedHash := <-calculatedHashChan

	if uploadedHash != calculatedHash {
		fmt.Println("Hash mismatch:", uploadedHash, " != ", calculatedHash)
		resultChan <- uploadResult{
			Index:    uploadInfo.Index,
			UploadId: uploadInfo.UploadId,
			Error:    fmt.Errorf("Hash mismatch for piece %d", uploadInfo.Index),
		}
		return
	}

	resultChan <- uploadResult{
		Index:    uploadInfo.Index,
		UploadId: uploadInfo.UploadId,
		MD5:      calculatedHash,
		Error:    nil,
	}
}

func (c *Client) uploadComplete(uploadId, destPath string, fileSize int64, blockHashes *[]string) error {
	blockListJSON, err := json.Marshal(*blockHashes)
	if err != nil {
		return fmt.Errorf("error marshaling block list: %v", err)
	}
	blockListJSONstr := string(blockListJSON)

	body, err := util.GetResponse(c.client.R().
		SetQueryParams(map[string]string{
			"isdir":    "0",
			"rtype":    "1",
			"bdstoken": c.bdsToken,
			"app_id":   appId,
			"jsToken":  c.jsToken,
		}).
		SetFormData(map[string]string{
			"path":        destPath,
			"size":        fmt.Sprintf("%d", fileSize),
			"uploadid":    uploadId,
			"target_path": c.cwd,
			"block_list":  blockListJSONstr,
		}).
		Post("/api/create"))
	if err != nil {
		return err
	}
	errNo := gjson.GetBytes(body, "errno").Int()
	errMsg := gjson.GetBytes(body, "errmsg").String()

	if errNo != 0 {
		return fmt.Errorf("Error uploading file: %s", errMsg)
	}
	return nil
}

type UploadFile struct {
	LocalPath string
	RemoteDir string
	FileName  string
}

func (c *Client) Upload(uploadFiles []UploadFile, opts *UploadOptions) error {
	if len(uploadFiles) == 0 {
		return fmt.Errorf("No files found to upload.\n")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := mpb.NewWithContext(
		ctx,
		mpb.WithOutput(os.Stderr),
		mpb.WithWidth(40),
		mpb.WithRefreshRate(180*time.Millisecond),
	)

	var wg sync.WaitGroup
	pieceChan := make(chan *uploadInfo)

	// Launch goroutines for uploading each chunk
	for range opts.MaxConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for info := range pieceChan {
				c.uploadChunk(ctx, p, info)
			}
		}()
	}

	for i, uploadFile := range uploadFiles {
		destPath := util.GetAbsPath(c.cwd, path.Join(uploadFile.RemoteDir, uploadFile.FileName))
		result, err := c.preCreate(uploadFile.LocalPath, uploadFile.RemoteDir, opts)
		if err != nil {
			return err
		}

		pieceCount := len(result.FilePieces)
		bar, _ := p.Add(
			int64(pieceCount),
			mpb.NopStyle().Build(),
			mpb.BarFillerClearOnComplete(),
			mpb.BarPriority(i),
			mpb.PrependDecorators(
				decor.Name(fmt.Sprintf("Uploading %s ... ", uploadFile.LocalPath)),
				decor.OnComplete(decor.Spinner(nil), "Done"),
				decor.OnComplete(decor.CountersNoUnit(" %d / %d"), ""),
			),
		)

		// Launch goroutine to collect upload results
		uploadResultChan := make(chan uploadResult, len(result.FilePieces))
		wg.Add(1)
		go func(
			uploadId string,
			chunkCount int,
			destPath string,
			fileSize int64,
			uploadResultChan chan uploadResult,
			spinner *mpb.Bar,
		) {
			defer wg.Done()
			defer close(uploadResultChan)

			blockHashes := make([]string, chunkCount)
			fileName := path.Base(destPath)
			for range blockHashes {
				select {
				case <-ctx.Done():
					return
				case res := <-uploadResultChan:
					if res.Error != nil {
						fmt.Fprintf(os.Stderr, "Error uploading %s piece %d: %v\n", fileName, res.Index, res.Error)
						return
					}
					blockHashes[res.Index] = res.MD5
					bar.Increment()
				}
			}
			err = c.uploadComplete(uploadId, destPath, fileSize, &blockHashes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error uploading %s: %v\n", fileName, err)
			}
		}(result.UploadId, pieceCount, destPath, result.FileSize, uploadResultChan, bar)

		// Send chunks to worker goroutines
		for j, piece := range result.FilePieces {
			args := &uploadInfo{
				Index:         j,
				LocalFilePath: uploadFile.LocalPath,
				TargetPath:    uploadFile.RemoteDir,
				FilePiece:     piece,
				UploadId:      result.UploadId,
				SenderChan:    uploadResultChan,
			}
			select {
			case <-ctx.Done():
				close(pieceChan)
				p.Wait()
				return fmt.Errorf("Upload cancelled")
			case pieceChan <- args:
			}
		}
	}
	close(pieceChan)
	p.Wait()

	return nil
}
