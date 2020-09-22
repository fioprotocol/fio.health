package fiohealth

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"log"
	"sort"
	"strings"
)

// S3Get pulls a file from S3
func S3Get(s3Bucket string, s3File string, region string) ([]byte, error) {
	buff := aws.NewWriteAtBuffer([]byte{})
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	downloader := s3manager.NewDownloader(sess)
	_, err := downloader.Download(buff, &s3.GetObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(s3File),
	})
	return buff.Bytes(), err
}

func S3GetUrl(s3Url string, optionalRegion string) ([]byte, error) {
	if optionalRegion == "" {
		optionalRegion = "us-east-1"
	}
	if !strings.HasPrefix(s3Url, "s3://") {
		return nil, errors.New("malformed s3 url")
	}
	var s3Bucket, s3File string
	parts := strings.Split(s3Url, "/")
	if len(parts) < 4 {
		return nil, errors.New("malformed s3 url")
	}
	s3Bucket = parts[2]
	s3File = strings.Join(parts[3:len(parts)], "/")
	return S3Get(s3Bucket, s3File, optionalRegion)
}

// S3Put writes a file to s3
func S3Put(s3Bucket string, s3File string, f []byte, region string) error {
	var contentType, maxAge string
	switch true {
	case strings.HasSuffix(s3File, ".html"):
		contentType = "text/html"
		maxAge = "max-age=120"
	case strings.HasSuffix(s3File, "index.json"), strings.HasSuffix(s3File, "report.json"):
		contentType = "application/json"
		maxAge = "max-age=120"
	case strings.HasSuffix(s3File, ".json"):
		contentType = "application/json"
		maxAge = "max-age=86400"
	case strings.HasSuffix(s3File, ".svg"):
		contentType = "image/svg+xml"
		maxAge = "max-age=86400"
	case strings.HasSuffix(s3File, ".css"):
		contentType = "text/css"
		maxAge = "max-age=86400"
	case strings.HasSuffix(s3File, ".js"):
		contentType = "application/javascript"
		maxAge = "max-age=120"
	}

	buff := bytes.NewBuffer(f)
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	uploader := s3manager.NewUploader(sess)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket:               aws.String(s3Bucket),
		Key:                  aws.String(s3File),
		Body:                 buff,
		ServerSideEncryption: aws.String("AES256"),
		ContentType:          aws.String(contentType),
		CacheControl:         aws.String(maxAge),
	})
	if err != nil {
		log.Printf("Could not save file: %v", err)
		return err
	}
	return nil
}

func CombineS3Report(report FinalResult, files []string, bucket string, prefix string, region string) []FinalResult {
	combined := make([]FinalResult, len(files)+1)
	combined[len(combined)-1] = report
	sort.Strings(files)
	for i := range files {
		j, err := S3GetUrl("s3://"+bucket+"/"+prefix+"/"+files[i], region)
		if err != nil {
			log.Println(err)
			continue
		}
		next := FinalResult{}
		err = json.Unmarshal(j, &next)
		if err != nil {
			log.Println(err)
			continue
		}
		combined[i] = next
	}
	return combined
}
