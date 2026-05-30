package store

import (
	"database/sql"
	"errors"
	"fmt"
	"mycontainer/consts"
	"mycontainer/container"
	"os"

	_ "modernc.org/sqlite"
)

const dbPath = consts.DefaultPath + "/state.db"

const initMigration = `
CREATE TABLE IF NOT EXISTS containers (
    id TEXT PRIMARY KEY,
    pid INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK(status IN ('created', 'running', 'error', 'completed')),
    lower_dir TEXT NOT NULL,
    upper_dir TEXT NOT NULL,
    work_dir TEXT NOT NULL,
    merged_dir TEXT NOT NULL,
    target_cmd TEXT NOT NULL,
    memory_limit INTEGER NOT NULL,
    memory_request INTEGER NOT NULL,
    cpu_limit INTEGER NOT NULL,
    cpu_period INTEGER NOT NULL,
    cpu_request INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

// InitDB гарантирует, что директория и таблица созданы
func InitDB() error {
	if err := os.MkdirAll(consts.DefaultPath, 0755); err != nil {
		return fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite db: %w", err)
	}
	defer db.Close()

	_, err = db.Exec(initMigration)
	if err != nil {
		return fmt.Errorf("failed to run init migration: %w", err)
	}

	return nil
}

// ExecuteInTransaction — оберточный метод для выполнения бизнес-логики
func ExecuteInTransaction(fn func(*sql.Tx) error) error {
	if err := InitDB(); err != nil {
		return err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite db: %w", err)
	}
	defer db.Close()

	// чтобы не блокировать другие программы
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to set WAL mode: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

// InsertContainer добавляет новый контейнер в базу данных.
func InsertContainer(c container.Container) error {
	return ExecuteInTransaction(func(tx *sql.Tx) error {
		query := `
		INSERT INTO containers (
			id, pid, status, lower_dir, upper_dir, work_dir, 
			merged_dir, target_cmd, memory_limit, cpu_limit, cpu_period, cpu_request, memory_request
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

		_, err := tx.Exec(query,
			c.ID, c.PID, c.Status, c.LowerDir, c.UpperDir, c.WorkDir,
			c.MergedDir, c.TargetCmd, c.MemoryLimit, c.CPULimit, c.CPUPeriod, c.CpuRequest, c.MemoryRequest,
		)
		if err != nil {
			return fmt.Errorf("failed to insert container %s: %w", c.ID, err)
		}
		return nil
	})
}

// UpdateContainerStatus обновляет только статус и PID контейнера (частый сценарий при запуске/остановке).
func UpdateContainerStatus(id string, pid int, status container.Status) error {
	return ExecuteInTransaction(func(tx *sql.Tx) error {
		query := `UPDATE containers SET pid = ?, status = ? WHERE id = ?;`

		result, err := tx.Exec(query, pid, status, id)
		if err != nil {
			return fmt.Errorf("failed to update container %s status: %w", id, err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return fmt.Errorf("container %s not found for update", id)
		}
		return nil
	})
}

// DeleteContainer удаляет контейнер из базы данных по его ID.
func DeleteContainer(id string) error {
	return ExecuteInTransaction(func(tx *sql.Tx) error {
		query := `DELETE FROM containers WHERE id = ?;`

		_, err := tx.Exec(query, id)
		if err != nil {
			return fmt.Errorf("failed to delete container %s: %w", id, err)
		}
		return nil
	})
}

// GetContainerByID находит контейнер по его ID.
func GetContainerByID(id string) (container.Container, error) {
	var c container.Container

	err := ExecuteInTransaction(func(tx *sql.Tx) error {
		query := `
		SELECT 
			id, pid, status, lower_dir, upper_dir, work_dir, 
			merged_dir, target_cmd, memory_limit, cpu_limit, cpu_period, created_at, cpu_request, memory_request
		FROM containers WHERE id = ?;`

		err := tx.QueryRow(query, id).Scan(
			&c.ID, &c.PID, &c.Status, &c.LowerDir, &c.UpperDir, &c.WorkDir,
			&c.MergedDir, &c.TargetCmd, &c.MemoryLimit, &c.CPULimit, &c.CPUPeriod, &c.CreatedAt, &c.CpuRequest, &c.MemoryRequest,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("container %s not found", id)
			}
			return err
		}
		return nil
	})

	return c, err
}

// GetAllContainers возвращает список всех контейнеров, зарегистрированных в базе данных.
// Контейнеры сортируются по времени создания (сначала новые).
func GetAllContainers() ([]container.Container, error) {
	var containers []container.Container

	err := ExecuteInTransaction(func(tx *sql.Tx) error {
		query := `
		SELECT 
			id, pid, status, lower_dir, upper_dir, work_dir, 
			merged_dir, target_cmd, memory_limit, cpu_limit, cpu_period, created_at, cpu_request, memory_request
		FROM containers
		ORDER BY created_at DESC;`

		rows, err := tx.Query(query)
		if err != nil {
			return fmt.Errorf("failed to execute select query: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var c container.Container
			err := rows.Scan(
				&c.ID, &c.PID, &c.Status, &c.LowerDir, &c.UpperDir, &c.WorkDir,
				&c.MergedDir, &c.TargetCmd, &c.MemoryLimit, &c.CPULimit, &c.CPUPeriod, &c.CreatedAt, &c.CpuRequest, &c.MemoryRequest,
			)
			if err != nil {
				return fmt.Errorf("failed to scan container row: %w", err)
			}
			containers = append(containers, c)
		}

		if err = rows.Err(); err != nil {
			return fmt.Errorf("error during rows iteration: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return containers, nil
}
