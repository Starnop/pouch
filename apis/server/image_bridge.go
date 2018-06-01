package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/pouch/apis/metrics"
	"github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/daemon/mgr"
	"github.com/alibaba/pouch/pkg/httputils"
	"github.com/alibaba/pouch/pkg/utils"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// pullImage will pull an image from a specified registry.
func (s *Server) pullImage(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	if utils.IsStale(ctx, req) {
		return s.pullImageStale(ctx, rw, req)
	}

	image := req.FormValue("fromImage")
	tag := req.FormValue("tag")

	if image == "" {
		err := fmt.Errorf("fromImage cannot be empty")
		return httputils.NewHTTPError(err, http.StatusBadRequest)
	}

	if tag != "" {
		image = image + ":" + tag
	}

	// record the time spent during image pull procedure.
	defer func(start time.Time) {
		metrics.ImagePullSummary.WithLabelValues(image).Observe(metrics.SinceInMicroseconds(start))
	}(time.Now())

	// get registry auth from Request header
	authStr := req.Header.Get("X-Registry-Auth")
	authConfig := types.AuthConfig{}
	if authStr != "" {
		data := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authStr))
		if err := json.NewDecoder(data).Decode(&authConfig); err != nil {
			return err
		}
	}
	// Error information has be sent to client, so no need call resp.Write
	if err := s.ImageMgr.PullImage(ctx, image, &authConfig, rw); err != nil {
		logrus.Errorf("failed to pull image %s: %v", image, err)
		return nil
	}
	return nil
}

type progressInfo struct {
	Ref       string
	Status    string
	Offset    int64
	Total     int64
	StartedAt time.Time
	UpdatedAt time.Time

	// For Error handling
	Code         int    // http response code
	ErrorMessage string // detail error information
}

type JSONError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type JSONProgress struct {
	terminalFd uintptr
	Current    int64 `json:"current,omitempty"`
	Total      int64 `json:"total,omitempty"`
	Start      int64 `json:"start,omitempty"`
}

type JSONMessage struct {
	Stream          string        `json:"stream,omitempty"`
	Status          string        `json:"status,omitempty"`
	Progress        *JSONProgress `json:"progressDetail,omitempty"`
	ProgressMessage string        `json:"progress,omitempty"` //deprecated
	ID              string        `json:"id,omitempty"`
	From            string        `json:"from,omitempty"`
	Time            int64         `json:"time,omitempty"`
	TimeNano        int64         `json:"timeNano,omitempty"`
	Error           *JSONError    `json:"errorDetail,omitempty"`
	ErrorMessage    string        `json:"error,omitempty"` //deprecated
	// Aux contains out-of-band data, such as digests for push signing.
	Aux *json.RawMessage `json:"aux,omitempty"`
}

// pullImageStale will pull an image from a specified registry.
func (s *Server) pullImageStale(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	image := req.FormValue("fromImage")
	tag := req.FormValue("tag")

	if image == "" {
		err := fmt.Errorf("fromImage cannot be empty")
		return httputils.NewHTTPError(err, http.StatusBadRequest)
	}

	if tag == "" {
		tag = "latest"
		if index := strings.LastIndex(image, ":"); index > 0 {
			tag = image[index+1:]
			image = image[:index]
		}
	}
	// record the time spent during image pull procedure.
	defer func(start time.Time) {
		metrics.ImagePullSummary.WithLabelValues(image + ":" + tag).Observe(metrics.SinceInMicroseconds(start))
	}(time.Now())

	// get registry auth from Request header
	authStr := req.Header.Get("X-Registry-Auth")
	authConfig := types.AuthConfig{}
	if authStr != "" {
		data := base64.NewDecoder(base64.URLEncoding, strings.NewReader(authStr))
		if err := json.NewDecoder(data).Decode(&authConfig); err != nil {
			return err
		}
	}

	///////////start/////////
	pipeR, pipeW := io.Pipe()
	retChan := make(chan error)
	go func() {
		dec := json.NewDecoder(pipeR)
		if _, err := dec.Token(); err != nil {
			retChan <- fmt.Errorf("failed to read the opening token: %v", err)
		}
		encoder := json.NewEncoder(rw)
		for dec.More() {
			var infos []progressInfo

			if err := dec.Decode(&infos); err != nil {
				retChan <- fmt.Errorf("failed to decode: %v", err)
				return
			}
			if len(infos) > 0 {
				for _, oneInfo := range infos {
					if oneInfo.ErrorMessage != "" {
						retChan <- fmt.Errorf("pull image %s error %s", image, oneInfo.ErrorMessage)
						return
					}
					switch oneInfo.Status {
					case "downloading", "uploading":
					case "resolving", "waiting":
						fallthrough
					default:
						continue
					}
					err := encoder.Encode(&JSONMessage{
						Status: oneInfo.Status,
						ID:     oneInfo.Ref,
						Progress: &JSONProgress{
							Current: oneInfo.Offset,
							Start:   0,
							Total:   oneInfo.Total,
						},
					})
					if err != nil {
						retChan <- err
					}
				}
			}
		}
		close(retChan)
	}()
	///////////end/////////

	// Error information has be sent to client, so no need call resp.Write
	if err := s.ImageMgr.PullImage(ctx, image+":"+tag, &authConfig, pipeW); err != nil {
		logrus.Errorf("failed to pull image %s:%s: %v", image, tag, err)
		return nil
	}
	pipeW.Close()

	return <-retChan
}

func (s *Server) getImage(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	idOrRef := mux.Vars(req)["name"]

	imageInfo, err := s.ImageMgr.GetImage(ctx, idOrRef)
	if err != nil {
		logrus.Errorf("failed to get image: %v", err)
		return err
	}

	return EncodeResponse(rw, http.StatusOK, imageInfo)
}

func (s *Server) listImages(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	filters := req.FormValue("filters")

	imageList, err := s.ImageMgr.ListImages(ctx, filters)
	if err != nil {
		logrus.Errorf("failed to list images: %v", err)
		return err
	}
	return EncodeResponse(rw, http.StatusOK, imageList)
}

func (s *Server) searchImages(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	searchPattern := req.FormValue("term")
	registry := req.FormValue("registry")

	searchResultItem, err := s.ImageMgr.SearchImages(ctx, searchPattern, registry)
	if err != nil {
		logrus.Errorf("failed to search images from resgitry: %v", err)
		return err
	}
	return EncodeResponse(rw, http.StatusOK, searchResultItem)
}

// removeImage deletes an image by reference.
func (s *Server) removeImage(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	name := mux.Vars(req)["name"]

	image, err := s.ImageMgr.GetImage(ctx, name)
	if err != nil {
		return err
	}

	containers, err := s.ContainerMgr.List(ctx, func(c *mgr.Container) bool {
		return c.Image == image.ID
	}, &mgr.ContainerListOption{All: true})
	if err != nil {
		return err
	}

	isForce := httputils.BoolValue(req, "force")
	if !isForce && len(containers) > 0 {
		return fmt.Errorf("Unable to remove the image %q (must force) - container %s is using this image", image.ID, containers[0].ID)
	}

	if err := s.ImageMgr.RemoveImage(ctx, name, isForce); err != nil {
		return err
	}

	rw.WriteHeader(http.StatusNoContent)
	return nil
}

// postImageTag adds tag for the existing image.
func (s *Server) postImageTag(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	name := mux.Vars(req)["name"]

	targetRef := req.FormValue("repo")
	if tag := req.FormValue("tag"); tag != "" {
		targetRef = fmt.Sprintf("%s:%s", targetRef, tag)
	}

	if err := s.ImageMgr.AddTag(ctx, name, targetRef); err != nil {
		return err
	}

	rw.WriteHeader(http.StatusCreated)
	return nil
}

// loadImage loads an image by http tar stream.
func (s *Server) loadImage(ctx context.Context, rw http.ResponseWriter, req *http.Request) error {
	imageName := req.FormValue("name")
	if imageName == "" {
		imageName = "unknown/unknown"
	}

	if err := s.ImageMgr.LoadImage(ctx, imageName, req.Body); err != nil {
		return err
	}

	rw.WriteHeader(http.StatusOK)
	return nil
}
