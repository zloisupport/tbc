package terabox

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	// MaxConcurrency is the number of chunks uploaded in parallel.
	MaxConcurrency int64
}

// pcsSliceSize returns the slice size the PCS server uses for a file of the
// given size. Files must be split into slices of exactly this size (the last
// slice may be smaller); the server rejects any other splitting with
// "Size of superfile2 first part should not be smaller than slice size"
// or "param err" when creating the file.
func pcsSliceSize(fileSize int64) (int64, error) {
	const mib = int64(1) << 20
	switch {
	case fileSize <= 4<<30:
		return 4 * mib, nil
	case fileSize <= 8<<30:
		return 8 * mib, nil
	case fileSize <= 16<<30:
		return 16 * mib, nil
	case fileSize <= 32<<30:
		return 32 * mib, nil
	case fileSize <= 64<<30:
		return 64 * mib, nil
	case fileSize <= 128<<30:
		return 128 * mib, nil
	}
	return 0, fmt.Errorf("file is too large for TeraBox (maximum 128 GiB)")
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

func (c *Client) preCreate(localPath, destination string) (*preCreateResult, error) {
	// obtain file info
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("error getting file info: %v", err)
	}
	fileSize := fileInfo.Size()
	filename := filepath.Base(localPath)

	remotePath := filepath.Join("/", destination, filename)

	chunkSize, err := pcsSliceSize(fileSize)
	if err != nil {
		return nil, err
	}

	var filePieces []*filePiece

	numChunks := (fileSize + chunkSize - 1) / chunkSize
	if numChunks == 0 {
		numChunks = 1
	}
	filePieces = make([]*filePiece, numChunks)
	blockList := make([]string, numChunks)

	for i := int64(0); i < numChunks; i++ {
		hashByte := md5.Sum(fmt.Appendf(nil, "%s_%03d", localPath, i))
		hashStr := hex.EncodeToString(hashByte[:])
		seek := chunkSize * i
		size := int64(math.Min(float64(fileSize-seek), float64(chunkSize)))

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
	util.Debugf("precreate response: %s", truncate(string(body), 2000))

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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// uploadEndpoint returns the PCS upload host for this TeraBox frontend.
// www uses c-all.<domain>, other frontends prefix it with their region
// (e.g. dm-c-all.terabox.com for dm.terabox.com).
func (c *Client) uploadEndpoint() string {
	prefix := c.region
	if prefix == "" {
		if parts := strings.Split(c.host, "."); len(parts) > 2 && parts[0] != "www" {
			prefix = parts[0]
		}
	}
	if prefix == "" || prefix == "www" {
		prefix = ""
	} else {
		prefix += "-"
	}
	return fmt.Sprintf("https://%sc-all.%s/rest/2.0/pcs/superfile2", prefix, c.domain)
}

// sendResult reports a chunk result without blocking after cancellation.
func sendResult(ctx context.Context, ch chan<- uploadResult, r uploadResult) {
	select {
	case ch <- r:
	case <-ctx.Done():
	}
}

func (c *Client) uploadChunk(ctx context.Context, pb *mpb.Progress, uploadInfo *uploadInfo) {
	resultChan := uploadInfo.SenderChan

	file, err := os.Open(uploadInfo.LocalFilePath)
	if err != nil {
		sendResult(ctx, resultChan, uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("failed to open file: %w", err)})
		return
	}
	defer file.Close()

	uploadEndpoint := c.uploadEndpoint()
	fileName := filepath.Base(uploadInfo.LocalFilePath)

	// prepare pipe
	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)
	hashReader := io.NewSectionReader(file, uploadInfo.FilePiece.Seek, uploadInfo.FilePiece.PieceSize)
	calculatedHashChan := make(chan string, 1)

	fileReader := io.NewSectionReader(file, uploadInfo.FilePiece.Seek, uploadInfo.FilePiece.PieceSize)

	// A byte-counting progress bar for an empty piece would never complete,
	// so zero-byte parts (empty files) are uploaded without one.
	var proxyReader io.Reader = fileReader
	if uploadInfo.FilePiece.PieceSize > 0 {
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
		if bar == nil {
			// The progress container was cancelled; the collector goroutine is
			// already unwinding, so there is nothing left to upload here.
			return
		}
		pr := bar.ProxyReader(fileReader)
		defer pr.Close()
		proxyReader = pr
	}

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
			if ctx.Err() == nil {
				fmt.Println("Error creating part:", err.Error())
			}
			return
		}

		_, err = io.Copy(fileWriter, proxyReader)
		if err != nil {
			if ctx.Err() == nil {
				fmt.Println("Error copying part:", err.Error())
			}
			return
		}
	}()

	u, err := url.Parse(uploadEndpoint)
	if err != nil {
		sendResult(ctx, resultChan, uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error parsing url: %w", err)})
		return
	}

	q := u.Query()
	q.Set("method", "upload")
	q.Set("type", "tmpfile")
	q.Set("app_id", appId)
	q.Set("channel", "dubox")
	q.Set("clienttype", "0")
	q.Set("web", "1")
	q.Set("path", fmt.Sprintf("%s/%s", strings.TrimSuffix(uploadInfo.TargetPath, "/"), fileName))
	q.Set("uploadid", uploadInfo.UploadId)
	q.Set("partseq", fmt.Sprintf("%d", uploadInfo.Index))
	u.RawQuery = q.Encode()
	util.Debugf("uploading piece %d to %s", uploadInfo.Index, u.String())

	req, err := http.NewRequest("POST", u.String(), pipeReader)
	if err != nil {
		sendResult(ctx, resultChan, uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error creating request: %w", err)})
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
		sendResult(ctx, resultChan, uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error uploading part: %w", err)})
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		sendResult(ctx, resultChan, uploadResult{Index: uploadInfo.Index, UploadId: uploadInfo.UploadId, Error: fmt.Errorf("error reading response: %w", err)})
		return
	}
	uploadedHash := gjson.GetBytes(body, "md5").String()
	calculatedHash := <-calculatedHashChan
	util.Debugf("upload piece %d response (HTTP %d): %s", uploadInfo.Index, res.StatusCode, truncate(string(body), 2000))

	if uploadedHash == "" {
		sendResult(ctx, resultChan, uploadResult{
			Index:    uploadInfo.Index,
			UploadId: uploadInfo.UploadId,
			Error: fmt.Errorf("upload of piece %d failed (HTTP %d, errno %d, errmsg %q): %s",
				uploadInfo.Index, res.StatusCode,
				gjson.GetBytes(body, "errno").Int(),
				gjson.GetBytes(body, "errmsg").String(),
				truncate(string(body), 300)),
		})
		return
	}

	if uploadedHash != calculatedHash {
		sendResult(ctx, resultChan, uploadResult{
			Index:    uploadInfo.Index,
			UploadId: uploadInfo.UploadId,
			Error: fmt.Errorf("hash mismatch for piece %d: server sent %q, local %q",
				uploadInfo.Index, uploadedHash, calculatedHash),
		})
		return
	}

	sendResult(ctx, resultChan, uploadResult{
		Index:    uploadInfo.Index,
		UploadId: uploadInfo.UploadId,
		MD5:      calculatedHash,
		Error:    nil,
	})
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

	var uploadDone []chan error

	// finish stops the workers, waits for every collector and closes the
	// progress bars. It must be called exactly once on every return path.
	finish := func(cause error) error {
		close(pieceChan)
		wg.Wait()

		var firstErr error
		for _, done := range uploadDone {
			err := <-done
			if err == nil || errors.Is(err, context.Canceled) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		p.Wait()

		if firstErr != nil {
			return fmt.Errorf("upload failed: %w", firstErr)
		}
		return cause
	}

	for i, uploadFile := range uploadFiles {
		destPath := util.GetAbsPath(c.cwd, path.Join(uploadFile.RemoteDir, uploadFile.FileName))
		result, err := c.preCreate(uploadFile.LocalPath, uploadFile.RemoteDir)
		if err != nil {
			return finish(err)
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
		done := make(chan error, 1)
		uploadDone = append(uploadDone, done)
		wg.Add(1)
		go func(
			uploadId string,
			chunkCount int,
			destPath string,
			fileSize int64,
			uploadResultChan chan uploadResult,
			spinner *mpb.Bar,
			done chan<- error,
		) {
			defer wg.Done()

			var uploadErr error
			defer func() { done <- uploadErr }()

			abortBar := func() {
				if spinner != nil {
					spinner.Abort(false)
				}
			}

			blockHashes := make([]string, chunkCount)
			fileName := path.Base(destPath)
			for range blockHashes {
				select {
				case <-ctx.Done():
					uploadErr = ctx.Err()
					abortBar()
					return
				case res := <-uploadResultChan:
					if res.Error != nil {
						fmt.Fprintf(os.Stderr, "Error uploading %s piece %d: %v\n", fileName, res.Index, res.Error)
						// Stop the remaining workers and finish the progress bar so
						// Upload does not deadlock in p.Wait().
						uploadErr = res.Error
						cancel()
						abortBar()
						return
					}
					blockHashes[res.Index] = res.MD5
					if spinner != nil {
						spinner.Increment()
					}
				}
			}
			uploadErr = c.uploadComplete(uploadId, destPath, fileSize, &blockHashes)
			if uploadErr != nil {
				fmt.Fprintf(os.Stderr, "Error uploading %s: %v\n", fileName, uploadErr)
			}
		}(result.UploadId, pieceCount, destPath, result.FileSize, uploadResultChan, bar, done)

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
				// Let the workers and collectors finish so no goroutine
				// outlives Upload and p.Wait() cannot deadlock.
				return finish(fmt.Errorf("upload cancelled"))
			case pieceChan <- args:
			}
		}
	}

	return finish(nil)
}
