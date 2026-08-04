package media

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2 implementa Uploader sobre Cloudflare R2, que habla el protocolo S3.
//
// El bucket es PRIVADO. Aquí viven pasaportes: el frontend nunca recibe una URL
// permanente, sino una firmada de vida corta emitida por el backend tras validar
// la sesión del administrador.
type R2 struct {
	client *s3.Client
	presign *s3.PresignClient
	bucket string
}

func NewR2(ctx context.Context, accountID, accessKey, secretKey, bucket string) (*R2, error) {
	if accountID == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("faltan credenciales de R2")
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)

	client := s3.New(s3.Options{
		// R2 ignora la región pero el SDK exige una; "auto" es la convención.
		Region:      "auto",
		BaseEndpoint: aws.String(endpoint),
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		// R2 no soporta el checksum por defecto que el SDK v2 añade desde 2025.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})

	return &R2{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}, nil
}

func (r *R2) Put(ctx context.Context, key, contentType string, data []byte) error {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return fmt.Errorf("PutObject %s: %w", key, err)
	}
	return nil
}

// SignedURL emite un enlace temporal. TTL corto por diseño: si alguien reenvía
// la URL de un pasaporte, caduca en minutos.
func (r *R2) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := r.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("firmando %s: %w", key, err)
	}
	return req.URL, nil
}
