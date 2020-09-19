package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/labstack/echo"
	"github.com/tenntenn/natureremo"
	"github.com/tktkc72/ouchidashboard/collector"
	"github.com/tktkc72/ouchidashboard/enum"
	"github.com/tktkc72/ouchidashboard/ouchi"
	"github.com/tktkc72/ouchidashboard/repository"
)

func getLogsHandler(c echo.Context) error {
	projectID := os.Getenv("GCP_PROJECT")
	rootPath := os.Getenv("FIRESTORE_ROOT_PATH")

	ctx := context.Background()
	firestoreClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Printf("failed to create firestore client due to: %v", err)
		return echo.ErrInternalServerError
	}
	defer firestoreClient.Close()

	roomName := c.Param("roomName")
	repository, err := repository.NewRepository(firestoreClient, rootPath, roomName, &collector.NowTime{})
	if err != nil {
		log.Printf("create repository: %v", err)
		if ouchi.IsNoRoom(err) {
			return echo.ErrBadRequest
		}
		return echo.ErrInternalServerError
	}

	logType, err := enum.ParseLogType(c.Param("logType"))
	if err != nil {
		log.Printf("failed to parse logtype: %s", c.Param("logType"))
		return echo.ErrBadRequest
	}

	service := ouchi.NewOuchi(repository)
	logs, err := service.GetLogs(roomName, logType, time.Now().AddDate(0, 0, -1), time.Now())
	if err != nil {
		return echo.ErrInternalServerError
	}
	return c.JSON(http.StatusOK, logs)
}

func collectorHandler(c echo.Context) error {
	accessToken := os.Getenv("NATURE_REMO_ACCESS_TOKEN")
	projectID := os.Getenv("GCP_PROJECT")
	rootPath := os.Getenv("FIRESTORE_ROOT_PATH")

	m := new(collector.Message)
	if err := c.Bind(m); err != nil {
		return err
	}
	errorChannel := make(chan error, len(m.RoomNames))
	for _, roomName := range m.RoomNames {
		go collect(accessToken, roomName, projectID, rootPath, errorChannel)
	}
	for range m.RoomNames {
		err := <-errorChannel
		if err != nil {
			log.Printf("collect: %v", err)
			if ouchi.IsNoRoom(err) {
				return echo.ErrBadRequest
			}
			return echo.ErrInternalServerError
		}
	}
	return nil
}

func collect(accessToken, roomName, projectID, rootPath string, c chan error) {
	natureremoClient := natureremo.NewClient(accessToken)
	fetcher := collector.NewFetcher(natureremoClient)

	ctx := context.Background()
	firestoreClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		c <- err
		return
	}
	defer firestoreClient.Close()
	repository, err := repository.NewRepository(firestoreClient, rootPath, roomName, &collector.NowTime{})
	if err != nil {
		c <- err
		return
	}

	service := collector.NewCollectorService(fetcher, repository)
	err = service.Collect()
	if err != nil {
		c <- err
		return
	}
	c <- nil
}
