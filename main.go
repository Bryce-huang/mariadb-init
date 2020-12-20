package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var POD_NAME = os.Getenv("MY_POD_NAME")

var REPLICAS = os.Getenv("REPLICAS")
var NS = os.Getenv("NS")

func main() {



	var valid = regexp.MustCompile("[0-9]{1,}")
	id, err := strconv.Atoi(valid.FindString("mariadb-galera-0"))
	if err != nil {
		log.Fatalf("无法转换字符串:%v", err)
	}
	fmt.Println(id)
	fmt.Println(filepath.Join("d", "data"))
	fmt.Println(getSeqNum())

}

func getSeqNum() (  int, error) {
	//	dat, err := os.Open("/bitnami/mariadb/data/grastate.dat")
	dat, err := os.Open("./grastate.dat")
	if err != nil {
		return math.MaxInt32, err
	}
	defer dat.Close()
	br := bufio.NewReader(dat)
	line := 0

	for {
		s, _, c := br.ReadLine()
		line++
		if c == io.EOF {
			break
		}
		if line != 4 {
			continue
		}
		str:=string(s)
		num, err := strconv.Atoi(strings.TrimSpace(str[strings.IndexByte(str,':')+1:]))
		if err != nil {
			return 0, err
		}
		return num,nil

	}
	return 0, nil
}
