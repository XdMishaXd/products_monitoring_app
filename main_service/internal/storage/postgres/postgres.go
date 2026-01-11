package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"main_service/internal/config"
	"main_service/internal/models"
	"main_service/internal/storage"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg *config.Config) (*PostgresRepo, error) {
	const op = "storage.storage.postgres.New"

	dsn := dsn(cfg)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to parse config: %w", op, err)
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = time.Minute * 30

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create pool: %w", op, err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%s: ping failed: %w", op, err)
	}

	return &PostgresRepo{pool: pool}, nil
}

// * SaveProduct добавляет продукт в базу данных
func (r *PostgresRepo) SaveProduct(
	ctx context.Context,
	userID int64,
	productURL, title string,
	marketplace models.Marketplace,
) (int64, error) {
	const op = "storage.postgres.SaveProduct"

	const query = `
		INSERT INTO products (user_id, url, title, marketplace)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	var id int64

	err := r.pool.QueryRow(ctx, query, userID, productURL, title, marketplace).Scan(&id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == storage.UniqueViolation {
			return 0, storage.ErrProductAlreadyExists
		}

		return 0, fmt.Errorf("%s: failed to save product: %w", op, err)
	}

	return id, nil
}

// * Products возвращает слайс продуктов для вывода пользователю
func (r *PostgresRepo) Products(ctx context.Context, userID, limit, offset int64) ([]models.Product, int64, error) {
	const op = "storage.Postgres.Products"

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			fmt.Printf("failed to rollback transaction: %v\n", err)
		}
	}()

	query := `
    SELECT id, title, marketplace, price, in_stock, last_checked, created_at, updated_at
      FROM products
      WHERE user_id = $1 AND price != -1
      ORDER BY created_at DESC
      LIMIT $2 OFFSET $3
  `

	rows, err := tx.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}

	products, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Product])
	if err != nil {
		return nil, 0, fmt.Errorf("%s: collect: %w", op, err)
	}

	var total int64

	countQuery := `SELECT COUNT(*) FROM products WHERE user_id = $1`

	err = tx.QueryRow(ctx, countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("%s: commit: %w", op, err)
	}

	return products, total, nil
}

// * ProductByID возвращает продукт по ID
func (r *PostgresRepo) ProductByID(ctx context.Context, productID int64) (models.Product, error) {
	const op = "storage.postgres.ProductByID"

	const query = `
		SELECT id, title, marketplace, price, in_stock, last_checked, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, productID)

	var p models.Product

	err := row.Scan(
		&p.ID,
		&p.Title,
		&p.Marketplace,
		&p.Price,
		&p.In_stock,
		&p.Last_checked,
		&p.Created_at,
		&p.Updated_at,
	)
	if err != nil {
		if p.Price == -1 {
			return models.Product{}, storage.ErrParsedProductNotYetRecieved
		}

		if errors.Is(err, pgx.ErrNoRows) {
			return models.Product{}, storage.ErrProductNotFound
		}

		return models.Product{}, fmt.Errorf("%s: failed to scan product: %w", op, err)
	}

	return p, nil
}

// * UpdateParsedData добавляет информацию о цене и наличии продукта
func (r *PostgresRepo) UpdateParsedData(ctx context.Context, productID int64, price float32, inStock bool) error {
	const op = "storage.postgres.UpdateParsedData"

	const query = `
		UPDATE products
		SET price = $1,
			in_stock = $2,
			last_checked = now(),
			updated_at = now()
		WHERE id = $3
	`

	cmd, err := r.pool.Exec(ctx, query, price, inStock, productID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cmd.RowsAffected() == 0 {
		return storage.ErrProductsNotFound
	}

	return nil
}

// * DeleteProduct удаляет продукт по productID и userID
func (r *PostgresRepo) DeleteProduct(ctx context.Context, productID, userID int64) error {
	const op = "storage.postgres.DeleteProduct"

	const query = `
		DELETE FROM products 
		WHERE id = $1 AND user_id = $2
	`

	cmd, err := r.pool.Exec(ctx, query, productID, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cmd.RowsAffected() == 0 {
		return storage.ErrProductNotFound
	}

	return nil
}

// * Close закрывает соединение с базой данных.
func (r *PostgresRepo) Close() {
	r.pool.Close()
}

// * dsn формирует конфигурацию базы данных.
func dsn(cfg *config.Config) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s database=%s sslmode=%s",
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.DBName,
		cfg.Postgres.SSLMode,
	)
}
