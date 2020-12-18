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
	getSeqNum()

}

func getSeqNum() (int, error) {
	//	dat, err := os.Open("/bitnami/mariadb/data/grastate.dat")
	dat, err := os.Open("./grastate.dat")
	if err != nil {
		return math.MaxInt32, err
	}
	defer dat.Close()
	br := bufio.NewReader(dat)
	for  {
		s, _, c := br.ReadLine()
		if c == io.EOF {
			break
		}

		fmt.Println(string(s))

	}
	return 0,nil

}
