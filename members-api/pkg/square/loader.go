package square

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/storage"
)

const cacheTTL = 10 * time.Minute

// Loader は GCS から _sqweb_session を読み込み、キャッシュした square.Client を返します。
// セッション更新後、最大 cacheTTL 以内に自動で新しいセッションを使い始めます。
type Loader struct {
	bucket string
	object string

	mu       sync.Mutex
	cached   *Client
	cachedAt time.Time
}

// NewLoader は GCS バケットとオブジェクト名を指定してローダーを作成します。
// bucket: GCS バケット名（SQUARE_SESSION_BUCKET 環境変数）
func NewLoader() *Loader {
	return &Loader{
		bucket: os.Getenv("SQUARE_SESSION_BUCKET"),
		object: "sqweb_session.txt",
	}
}

// Get はキャッシュ済みクライアントを返します。TTL 切れの場合は GCS から再読み込みします。
// GCS 読み込みに失敗した場合はキャッシュを継続使用します（フォールバック）。
func (l *Loader) Get() (*Client, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cached != nil && time.Since(l.cachedAt) < cacheTTL {
		return l.cached, nil
	}

	client, err := l.load()
	if err != nil {
		if l.cached != nil {
			// GCS 障害時はキャッシュで継続
			return l.cached, nil
		}
		return nil, fmt.Errorf("square session load: %w", err)
	}

	l.cached = client
	l.cachedAt = time.Now()
	return client, nil
}

func (l *Loader) load() (*Client, error) {
	if l.bucket == "" {
		return nil, fmt.Errorf("SQUARE_SESSION_BUCKET が未設定です")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gcs, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer gcs.Close()

	r, err := gcs.Bucket(l.bucket).Object(l.object).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return NewClient(string(b), os.Getenv("SQUARE_API_TOKEN"))
}
