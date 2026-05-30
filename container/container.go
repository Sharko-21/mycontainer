package container

import (
	"fmt"
	"os"
	"path/filepath"

	"crypto/rand"
	"encoding/hex"
)

/*
upper/ (Слой записи): Изначально эта папка абсолютно пустая. Как только контейнер запускается,
любые действия, связанные с изменением файловой системы
(создание файлов, удаление, редактирование конфигов, установка пакетов через apk add),
физически записываются только сюда. Исходный образ (lower) ядро Linux не трогает.

work/ (Служебная папка): Она тоже должна быть пустой.
Она используется самой файловой системой ядра Linux для обеспечения атомарности операций
(например, когда файл подготавливается к созданию или удалению, ядро сначала делает это в work,
а потом атомарно переносит в upper).
Критическое требование ядра Linux: папка work обязана находиться на том же диске (файловой системе) и в той же
родительской директории, что и upper.

merged/ (Точка слияния): Изначально пустая папка, которая используется как обычная точка монтирования (mount point).
Когда хост выполняет системный вызов syscall.Mount("overlay", merged, ...),
ядро виртуально объединяет содержимое lower и upper, показывая результат в merged.
Именно эту папку мы затем делаем новым корнем / для контейнера.
*/
func CreateContainerDirs(containerID string) (upper, work, merged string, err error) {
	baseContainerDir := filepath.Join("/var/lib/mycontainer/containers", containerID)
	upper = filepath.Join(baseContainerDir, "upper")
	work = filepath.Join(baseContainerDir, "work")
	merged = filepath.Join(baseContainerDir, "merged")
	err = os.MkdirAll(upper, 0777)
	if err != nil {
		return
	}
	err = os.MkdirAll(work, 0777)
	if err != nil {
		return
	}
	err = os.MkdirAll(merged, 0777)
	if err != nil {
		return
	}
	return
}

func GenerateContainerID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func RemoveContainerDirs(containerID string) error {
	baseContainerDir := filepath.Join("/var/lib/mycontainer/containers", containerID)
	return os.RemoveAll(baseContainerDir)
}
