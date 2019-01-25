package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
)

var (
	defKey = []byte{0}
)

type pidlist []int

func main() {
	db, err := bolt.Open("/tmp/mck.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	for {
		users := findUserWithMinecraft()
		fmt.Println(users)
		for owner, pids := range users {
			err = incTime(db, owner)
			if err != nil {
				log.Printf("Can inc user runtime: %s\n", err)
				continue
			}
			runtime, err := getUserRuntime(db, owner)
			if err != nil {
				log.Printf("Can check user runtime: %s\n", err)
				continue
			}
			if runtime > 2 {
				killAll(pids)
			}
		}
		time.Sleep(1 * time.Minute)
	}
}

func killAll(pids pidlist) {
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err != nil {
			log.Printf("Can't kill %d\n", pid)
		}
		proc.Kill()
	}
}

func getUserRuntime(db *bolt.DB, user uint32) (uint64, error) {
	indexName := fmt.Sprintf("user_%d", user)
	var runtime uint64
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(indexName))
		key := []byte(fmt.Sprintf("date_%s", time.Now().Format("060102")))
		buf := b.Get(key)
		runtime = binary.BigEndian.Uint64(buf)
		return nil
	})
	return runtime, err
}

func incTime(db *bolt.DB, user uint32) error {
	indexName := fmt.Sprintf("user_%d", user)
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(indexName))
		if err != nil {
			return err
		}
		key := []byte(fmt.Sprintf("date_%s", time.Now().Format("060102")))
		buf := b.Get(key)
		var runtime uint64 = 1
		if buf != nil {
			runtime = binary.BigEndian.Uint64(buf) + 1
		}
		return b.Put(key, itob(runtime))
	})

}

func findUserWithMinecraft() map[uint32]pidlist {
	users := make(map[uint32]pidlist)
	pids, _ := findTLauncherProcesses()
	for _, pid := range pids {
		owner, err := getProcessOwner(pid)
		if err != nil {
			log.Printf("Can't get linux file info: %d\n", pid)
			continue
		}
		users[owner] = append(users[owner], pid)
	}
	return users
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func getProcessOwner(pid int) (uint32, error) {
	fi, err := os.Lstat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return 0, err
	}
	linuxInfo, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, err
	}
	return linuxInfo.Uid, nil
}

func findTLauncherProcesses() ([]int, error) {
	files, err := ioutil.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		pid, err := strconv.ParseInt(file.Name(), 10, 0)
		if err != nil {
			continue
		}

		exe, err := os.Readlink(fmt.Sprintf("/proc/%s/exe", file.Name()))
		if err != nil {
			continue
		}

		if exe[(len(exe)-4):] == "java" {
			fileBytes, err := ioutil.ReadFile(fmt.Sprintf("/proc/%s/cmdline", file.Name()))
			if err != nil {
				continue
			}
			content := string(fileBytes)
			if strings.Contains(content, "minecraft.launcher.brand=minecraft-launcher") {
				pids = append(pids, int(pid))
			}
		}
	}
	return pids, nil
}
