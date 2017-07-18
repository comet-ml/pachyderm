package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/pachyderm/pachyderm/src/client"
	"github.com/pachyderm/pachyderm/src/client/pfs"
	"github.com/pachyderm/pachyderm/src/client/pkg/grpcutil"
	"github.com/pachyderm/pachyderm/src/server/pkg/obj"

	protolion "go.pedge.io/lion/proto"
	"go.pedge.io/proto/rpclog"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	grpcErrorf = grpc.Errorf // needed to get passed govet
)

const (
	// The maximum number of items we log in response to a List* API
	maxListItemsLog = 10
)

type apiServer struct {
	protorpclog.Logger
	driver *driver
}

func newLocalAPIServer(address string, etcdPrefix string) (*apiServer, error) {
	d, err := newLocalDriver(address, etcdPrefix)
	if err != nil {
		return nil, err
	}
	return &apiServer{
		Logger: protorpclog.NewLogger("pfs.API"),
		driver: d,
	}, nil
}

func newAPIServer(address string, etcdAddresses []string, etcdPrefix string, cacheBytes int64) (*apiServer, error) {
	d, err := newDriver(address, etcdAddresses, etcdPrefix, cacheBytes)
	if err != nil {
		return nil, err
	}
	return &apiServer{
		Logger: protorpclog.NewLogger("pfs.API"),
		driver: d,
	}, nil
}

func (a *apiServer) CreateRepo(ctx context.Context, request *pfs.CreateRepoRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if err := a.driver.createRepo(ctx, request.Repo, request.Provenance, request.Description, request.Update); err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) InspectRepo(ctx context.Context, request *pfs.InspectRepoRequest) (response *pfs.RepoInfo, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	return a.driver.inspectRepo(ctx, request.Repo)
}

func (a *apiServer) ListRepo(ctx context.Context, request *pfs.ListRepoRequest) (response *pfs.RepoInfos, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	repoInfos, err := a.driver.listRepo(ctx, request.Provenance)
	return &pfs.RepoInfos{RepoInfo: repoInfos}, err
}

func (a *apiServer) DeleteRepo(ctx context.Context, request *pfs.DeleteRepoRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if request.All {
		if err := a.driver.deleteAll(ctx); err != nil {
			return nil, err
		}
	} else {
		if err := a.driver.deleteRepo(ctx, request.Repo, request.Force); err != nil {
			return nil, err
		}
	}

	return &types.Empty{}, nil
}

func (a *apiServer) StartCommit(ctx context.Context, request *pfs.StartCommitRequest) (response *pfs.Commit, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	commit, err := a.driver.startCommit(ctx, request.Parent, request.Branch, request.Provenance)
	if err != nil {
		return nil, err
	}
	return commit, nil
}

func (a *apiServer) BuildCommit(ctx context.Context, request *pfs.BuildCommitRequest) (response *pfs.Commit, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	commit, err := a.driver.buildCommit(ctx, request.Parent, request.Branch, request.Provenance, request.Tree)
	if err != nil {
		return nil, err
	}
	return commit, nil
}

func (a *apiServer) FinishCommit(ctx context.Context, request *pfs.FinishCommitRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if err := a.driver.finishCommit(ctx, request.Commit); err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) InspectCommit(ctx context.Context, request *pfs.InspectCommitRequest) (response *pfs.CommitInfo, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	return a.driver.inspectCommit(ctx, request.Commit)
}

func (a *apiServer) ListCommit(ctx context.Context, request *pfs.ListCommitRequest) (response *pfs.CommitInfos, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	commitInfos, err := a.driver.listCommit(ctx, request.Repo, request.To, request.From, request.Number)
	if err != nil {
		return nil, err
	}
	return &pfs.CommitInfos{
		CommitInfo: commitInfos,
	}, nil
}

func (a *apiServer) ListBranch(ctx context.Context, request *pfs.ListBranchRequest) (response *pfs.BranchInfos, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	branches, err := a.driver.listBranch(ctx, request.Repo)
	if err != nil {
		return nil, err
	}
	return &pfs.BranchInfos{BranchInfo: branches}, nil
}

func (a *apiServer) SetBranch(ctx context.Context, request *pfs.SetBranchRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if err := a.driver.setBranch(ctx, request.Commit, request.Branch); err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) DeleteBranch(ctx context.Context, request *pfs.DeleteBranchRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if err := a.driver.deleteBranch(ctx, request.Repo, request.Branch); err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) DeleteCommit(ctx context.Context, request *pfs.DeleteCommitRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if err := a.driver.deleteCommit(ctx, request.Commit); err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) FlushCommit(request *pfs.FlushCommitRequest, stream pfs.API_FlushCommitServer) (retErr error) {
	ctx := stream.Context()
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, nil, retErr, time.Since(start)) }(time.Now())

	commitStream, err := a.driver.flushCommit(ctx, request.Commits, request.ToRepos)
	if err != nil {
		return err
	}
	defer func() {
		commitStream.Close()
	}()

	for {
		ev, ok := <-commitStream.Stream()
		if !ok {
			return nil
		}
		if ev.Err != nil {
			return ev.Err
		}
		if err := stream.Send(ev.Value); err != nil {
			return err
		}
	}
}

func (a *apiServer) SubscribeCommit(request *pfs.SubscribeCommitRequest, stream pfs.API_SubscribeCommitServer) (retErr error) {
	ctx := stream.Context()
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, nil, retErr, time.Since(start)) }(time.Now())

	commitStream, err := a.driver.subscribeCommit(ctx, request.Repo, request.Branch, request.From)
	if err != nil {
		return err
	}
	defer func() {
		commitStream.Close()
	}()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case ev, ok := <-commitStream.Stream():
			if !ok {
				return nil
			}
			if ev.Err != nil {
				return ev.Err
			}
			if err := stream.Send(ev.Value); err != nil {
				return err
			}
		}
	}
}

func (a *apiServer) PutFile(putFileServer pfs.API_PutFileServer) (retErr error) {
	ctx := putFileServer.Context()
	defer drainFileServer(putFileServer)
	defer func() {
		if err := putFileServer.SendAndClose(&types.Empty{}); err != nil && retErr == nil {
			retErr = err
		}
	}()
	request, err := putFileServer.Recv()
	if err != nil && err != io.EOF {
		return err
	}
	if err == io.EOF {
		// tolerate people calling and immediately hanging up
		return nil
	}
	// We remove request.Value from the logs otherwise they would be too big.
	func() {
		requestValue := request.Value
		request.Value = nil
		a.Log(request, nil, nil, 0)
		request.Value = requestValue
	}()
	defer func(start time.Time) {
		requestValue := request.Value
		request.Value = nil
		a.Log(request, nil, retErr, time.Since(start))
		request.Value = requestValue
	}(time.Now())
	// not cleaning the path can result in weird effects like files called
	// ./foo which won't display correctly when the filesystem is mounted
	request.File.Path = path.Clean(request.File.Path)
	var r io.Reader
	if request.Url != "" {
		url, err := url.Parse(request.Url)
		if err != nil {
			return err
		}
		switch url.Scheme {
		case "http":
			fallthrough
		case "https":
			resp, err := http.Get(request.Url)
			if err != nil {
				return err
			}
			defer func() {
				if err := resp.Body.Close(); err != nil && retErr == nil {
					retErr = err
				}
			}()
			r = resp.Body
		case "pfs":
			return a.putFilePfs(ctx, request, url)
		default:
			objClient, err := obj.NewClientFromURLAndSecret(putFileServer.Context(), request.Url)
			if err != nil {
				return err
			}
			return a.putFileObj(ctx, objClient, request, url)
		}
	} else {
		reader := putFileReader{
			server: putFileServer,
		}
		_, err = reader.buffer.Write(request.Value)
		if err != nil {
			return err
		}
		r = &reader
	}
	if err := a.driver.putFile(ctx, request.File, request.Delimiter, request.TargetFileDatums, request.TargetFileBytes, r); err != nil {
		return err
	}
	return nil
}

func (a *apiServer) putFilePfs(ctx context.Context, request *pfs.PutFileRequest, url *url.URL) error {
	pClient, err := client.NewFromAddress(url.Host)
	if err != nil {
		return err
	}
	put := func(outPath string, inRepo string, inCommit string, inFile string) (retErr error) {
		r, err := pClient.GetFileReader(inRepo, inCommit, inFile, 0, 0)
		if err != nil {
			return err
		}
		return a.driver.putFile(ctx, client.NewFile(request.File.Commit.Repo.Name, request.File.Commit.ID, outPath), request.Delimiter, request.TargetFileDatums, request.TargetFileBytes, r)
	}
	splitPath := strings.Split(strings.TrimPrefix(url.Path, "/"), "/")
	if len(splitPath) < 2 {
		return fmt.Errorf("pfs put-file path must be of form repo/commit[/path/to/file] got: %s", url.Path)
	}
	repo := splitPath[0]
	commit := splitPath[1]
	file := ""
	if len(splitPath) >= 3 {
		file = filepath.Join(splitPath[2:]...)
	}
	if request.Recursive {
		var eg errgroup.Group
		if err := pClient.Walk(splitPath[0], commit, file, func(fileInfo *pfs.FileInfo) error {
			if fileInfo.FileType != pfs.FileType_FILE {
				return nil
			}
			eg.Go(func() error {
				return put(filepath.Join(request.File.Path, strings.TrimPrefix(fileInfo.File.Path, file)), repo, commit, fileInfo.File.Path)
			})
			return nil
		}); err != nil {
			return err
		}
		return eg.Wait()
	}
	return put(request.File.Path, repo, commit, file)
}

func (a *apiServer) putFileObj(ctx context.Context, objClient obj.Client, request *pfs.PutFileRequest, url *url.URL) (retErr error) {
	put := func(ctx context.Context, filePath string, objPath string) error {
		logRequest := &pfs.PutFileRequest{
			Delimiter: request.Delimiter,
			Url:       objPath,
			File: &pfs.File{
				Path: filePath,
			},
			Recursive: request.Recursive,
		}
		protorpclog.Log("pfs.API", "putFileObj", logRequest, nil, nil, 0)
		defer func(start time.Time) {
			protorpclog.Log("pfs.API", "putFileObj", logRequest, nil, retErr, time.Since(start))
		}(time.Now())
		r, err := objClient.Reader(objPath, 0, 0)
		if err != nil {
			return err
		}
		defer func() {
			if err := r.Close(); err != nil && retErr == nil {
				retErr = err
			}
		}()
		return a.driver.putFile(ctx, client.NewFile(request.File.Commit.Repo.Name, request.File.Commit.ID, filePath),
			request.Delimiter, request.TargetFileDatums, request.TargetFileBytes, r)
	}
	if request.Recursive {
		eg, egContext := errgroup.WithContext(ctx)
		path := strings.TrimPrefix(url.Path, "/")
		sem := make(chan struct{}, client.DefaultMaxConcurrentStreams)
		objClient.Walk(path, func(name string) error {
			eg.Go(func() error {
				sem <- struct{}{}
				defer func() {
					<-sem
				}()

				if strings.HasSuffix(name, "/") {
					// Amazon S3 supports objs w keys that end in a '/'
					// PFS needs to treat such a key as a directory.
					// In this case, we rely on the driver PutFile to
					// construct the 'directory' diffs from the file prefix
					protolion.Warnf("ambiguous key %v, not creating a directory or putting this entry as a file", name)
					return nil
				}
				return put(egContext, filepath.Join(request.File.Path, strings.TrimPrefix(name, path)), name)
			})
			return nil
		})
		return eg.Wait()
	}
	// Joining Host and Path to retrieve the full path after "scheme://"
	return put(ctx, request.File.Path, path.Join(url.Host, url.Path))
}

func (a *apiServer) GetFile(request *pfs.GetFileRequest, apiGetFileServer pfs.API_GetFileServer) (retErr error) {
	ctx := apiGetFileServer.Context()
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, nil, retErr, time.Since(start)) }(time.Now())

	file, err := a.driver.getFile(ctx, request.File, request.OffsetBytes, request.SizeBytes)
	if err != nil {
		return err
	}
	return grpcutil.WriteToStreamingBytesServer(file, apiGetFileServer)
}

func (a *apiServer) InspectFile(ctx context.Context, request *pfs.InspectFileRequest) (response *pfs.FileInfo, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	return a.driver.inspectFile(ctx, request.File)
}

func (a *apiServer) ListFile(ctx context.Context, request *pfs.ListFileRequest) (response *pfs.FileInfos, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) {
		if response != nil && len(response.FileInfo) > maxListItemsLog {
			protolion.Infof("Response contains %d objects; logging the first %d", len(response.FileInfo), maxListItemsLog)
			a.Log(request, &pfs.FileInfos{response.FileInfo[:maxListItemsLog]}, retErr, time.Since(start))
		} else {
			a.Log(request, response, retErr, time.Since(start))
		}
	}(time.Now())

	fileInfos, err := a.driver.listFile(ctx, request.File)
	if err != nil {
		return nil, err
	}
	return &pfs.FileInfos{
		FileInfo: fileInfos,
	}, nil
}

func (a *apiServer) GlobFile(ctx context.Context, request *pfs.GlobFileRequest) (response *pfs.FileInfos, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) {
		if response != nil && len(response.FileInfo) > maxListItemsLog {
			protolion.Infof("Response contains %d objects; logging the first %d", len(response.FileInfo), maxListItemsLog)
			a.Log(request, &pfs.FileInfos{response.FileInfo[:maxListItemsLog]}, retErr, time.Since(start))
		} else {
			a.Log(request, response, retErr, time.Since(start))
		}
	}(time.Now())

	fileInfos, err := a.driver.globFile(ctx, request.Commit, request.Pattern)
	if err != nil {
		return nil, err
	}
	return &pfs.FileInfos{
		FileInfo: fileInfos,
	}, nil
}

func (a *apiServer) DiffFile(ctx context.Context, request *pfs.DiffFileRequest) (response *pfs.DiffFileResponse, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) {
		if response != nil && (len(response.NewFiles) > maxListItemsLog || len(response.OldFiles) > maxListItemsLog) {
			protolion.Infof("Response contains too many objects; truncating.")
			a.Log(request, &pfs.DiffFileResponse{
				NewFiles: truncateFiles(response.NewFiles),
				OldFiles: truncateFiles(response.OldFiles),
			}, retErr, time.Since(start))
		} else {
			a.Log(request, response, retErr, time.Since(start))
		}
	}(time.Now())
	newFileInfos, oldFileInfos, err := a.driver.diffFile(ctx, request.NewFile, request.OldFile)
	if err != nil {
		return nil, err
	}
	return &pfs.DiffFileResponse{
		NewFiles: newFileInfos,
		OldFiles: oldFileInfos,
	}, nil
}

func (a *apiServer) DeleteFile(ctx context.Context, request *pfs.DeleteFileRequest) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	err := a.driver.deleteFile(ctx, request.File)
	if err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) DeleteAll(ctx context.Context, request *types.Empty) (response *types.Empty, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	if err := a.driver.deleteAll(ctx); err != nil {
		return nil, err
	}
	return &types.Empty{}, nil
}

func (a *apiServer) ListDatum(ctx context.Context, request *pfs.ListDatumRequest) (response *pfs.DatumInfos, retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, response, retErr, time.Since(start)) }(time.Now())

	// Check 'stats' branch exists
	repo := &pfs.Repo{
		Name: request.PipelineName,
	}
	branches, err := a.driver.listBranch(ctx, repo)
	if err != nil {
		return nil, err
	}
	var statsBranch *pfs.BranchInfo
	for _, branch := range branches {
		if branch.Name == "stats" {
			statsBranch = branch
		}
	}
	if statsBranch == nil {
		return nil, fmt.Errorf("stats branch not found on %v", repo.Name)
	}
	// List the files under /jobID to get all the datums
	file := &pfs.File{
		Commit: statsBranch.Head,
		Path:   fmt.Sprintf("/%v", request.JobId),
	}
	allFileInfos, err := a.driver.listFile(ctx, file)
	if err != nil {
		return nil, err
	}
	var datumInfos []*pfs.DatumInfo
	datums := make(map[string]*pfs.DatumInfo)
	for _, fileInfo := range allFileInfos {
		_, datumHash := filepath.Split(fileInfo.File.Path)
		datums[datumHash] = &pfs.DatumInfo{
			Hash:  []byte(datumHash),
			State: pfs.DatumState_DATUM_SKIPPED,
		}
	}

	// Diff the files under /parentJobID and /jobID to get non-skipped datums
	newFile := &pfs.File{
		Commit: statsBranch.Head,
		Path:   fmt.Sprintf("/%v", request.JobId),
	}
	oldFile := &pfs.File{
		Commit: statsBranch.Head.ParentCommit,
		Path:   fmt.Sprintf("/%v", request.JobId),
	}
	newFileInfos, oldFileInfos, err := a.driver.diffFile(ctx, newFile, oldFile)
	if err != nil {
		return nil, err
	}
	fmt.Printf("# new %v, # old %v\n", len(newFileInfos), len(oldfileInfos))

	// The newFileInfos contain datums that were new in this job
	// For these datums, populate the status and stats
	for _, fileInfo := range newFileInfos {
		_, datumHash := filepath.Split(fileInfo.File.Path)
		status, err = a.getDatumStatus(statsBranch.Head, request.JobId, datumHash)
		if err != nil {
			return nil, err
		}
		datums[datumHash].Status = status
		status, err = a.getDatumStats(statsBranch.Head, request.JobId, datumHash)
		if err != nil {
			return nil, err
		}
		datums[datumHash].Stats = stats
	}

	// Sort results (failed first, slow first)
	// TODO

	return &pfs.DatumInfos{datumInfos}, nil
}

func (a *apiServer) getDatumStatus(commit *pfs.Commit, jobID string, datumHash string) (*pfs.DatumStatus, err) {
	//return grpcutil.WriteToStreamingBytesServer(file, apiGetFileServer)
	return nil, fmt.Errorf("not implemented")
}

func (a *apiServer) getDatumStats(commit *pfs.Commit, jobID string, datumHash string) (*pfs.DatumStatus, err) {
	file := &pfs.File{
		Commit: commit,
		Path:   fmt.Sprintf("/%v/%v/stats", jobID, datumHash),
	}
	file, err := a.driver.getFile(ctx, file, request.OffsetBytes, request.SizeBytes)
	if err != nil {
		return err
	}
	return nil, fmt.Errorf("not implemented")
}

func (a *apiServer) InspectDatum(ctx context.Context, request *pfs.InspectDatumRequest) (response *pfs.DatumInfo, retErr error) {
	return nil, fmt.Errorf("not implemented")
}

type putFileReader struct {
	server pfs.API_PutFileServer
	buffer bytes.Buffer
}

func (r *putFileReader) Read(p []byte) (int, error) {
	if r.buffer.Len() == 0 {
		request, err := r.server.Recv()
		if err != nil {
			return 0, err
		}
		//buffer.Write cannot error
		r.buffer.Write(request.Value)
	}
	return r.buffer.Read(p)
}

func (a *apiServer) getVersion(ctx context.Context) (int64, error) {
	md, ok := metadata.FromContext(ctx)
	if !ok {
		return 0, fmt.Errorf("version not found in context")
	}
	encodedVersion, ok := md["version"]
	if !ok {
		return 0, fmt.Errorf("version not found in context")
	}
	if len(encodedVersion) != 1 {
		return 0, fmt.Errorf("version not found in context")
	}
	return strconv.ParseInt(encodedVersion[0], 10, 64)
}

func drainFileServer(putFileServer interface {
	Recv() (*pfs.PutFileRequest, error)
}) {
	for {
		if _, err := putFileServer.Recv(); err != nil {
			break
		}
	}
}

func truncateFiles(fileInfos []*pfs.FileInfo) []*pfs.FileInfo {
	if len(fileInfos) > maxListItemsLog {
		return fileInfos[:maxListItemsLog]
	}
	return fileInfos
}
