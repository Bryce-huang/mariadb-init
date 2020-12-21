package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var PodName = os.Getenv("MY_POD_NAME")
var REPLICAS = os.Getenv("REPLICAS")
var MariadbGaleraClusterAddress = os.Getenv("MARIADB_GALERA_CLUSTER_ADDRESS")
var signal = make(chan int)

const (
	GrastatePath = "/bitnami/mariadb/data/grastate.dat"
	MysqlPort    = "3306"
	InitPort     = 3307
)

func main() {
	go runHttp()
	go initToWait()

	// 如果文件存在则需要确保所有节点都正常通信过一次
	os.Exit(<-signal)
}

func initToWait() {
	//  检查是否存在 grastate文件
	// 不存在 .检查本pod是不是id 为0，直接启动，不是的话 检查上一个node节点是否活跃，否则等待。直到上一个节点活跃
	file, err := os.Open(GrastatePath)
	if os.IsNotExist(err) {
		fmt.Println("文件不存在,按照顺序启动。。。")
		if isFirst() {
			signal <- 0
			return
		} else {
			for {
				if preNodeReady(getPodNum() - 1) {
					signal <- 0
					return
				}
				time.Sleep(time.Second * 10)
			}
		}
	}
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// 存在
	// 判断集群是否存在  存在集群，直接启动本机
	for i := 0; i < 3; i++ {
		if isOpen(MariadbGaleraClusterAddress, MysqlPort) {
			signal <- 0
			return
		}
	}

	// 获取所有seqnum，确保所有节点都至少通信过一次，检查是否所有的都是一样的seqnum，

	seqNums := getAllSeqNum()

	// 一样，按照顺序启动
	if isAllSeqNoEqual(seqNums) {
		if isFirst() {
			signal <- 0
			return
		} else {
			for {
				if preNodeReady(getPodNum() - 1) {
					signal <- 0
					return
				}
				time.Sleep(time.Second * 10)
			}
		}
	}

	// 不一样，确定seqno从大到小 依次启动
	sort.Slice(seqNums, func(i, j int) bool {
		return seqNums[i].num > seqNums[j].num
	})

	if meIsMax(seqNums) {
		signal <- 0
		return
	}
	pre := getPreNum(seqNums)
	if pre == -1 {
		log.Printf("fail to judge，the slice is %v", seqNums)
		signal <- 0
		return
	}

	for {
		if preNodeReady(getPodNum() - 1) {
			signal <- 0
			return
		}
		time.Sleep(time.Second * 10)
	}
}

func meIsMax(seqNums []SeqNum) bool {
	if seqNums[0].node == getPodNum() {
		return true
	}
	return false
}

func getPreNum(seqNums []SeqNum) int {
	node := getPodNum()
	for i, v := range seqNums {
		if v.node == node {
			return i - 1
		}
	}
	return -1
}

func isAllSeqNoEqual(seqNums []SeqNum) bool {
	key := map[int]int{}
	for _, v := range seqNums {
		key[v.num]++
	}
	if len(key) == 1 {
		return true
	}
	return false
}

type SeqNum struct {
	node int
	num  int
}

func getAllSeqNum() []SeqNum {
	replicas, err := strconv.Atoi(REPLICAS)
	if err != nil {
		panic(err)
	}
	prefix := getPodPrefix()
	seqNums := make([]SeqNum, replicas)
	for i := 0; i < replicas; i++ {
		index := strconv.Itoa(i)
		url := prefix + index + ":3307/seq-num?id=" + strconv.Itoa(getPodNum())
		resp, err := http.Get(url)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		var result Resp
		_ = json.Unmarshal(body, &result)
		seqNums = append(seqNums, SeqNum{i, result.Data})
	}

	return seqNums

}

func runHttp() {
	router := gin.Default()
	router.GET("/seq-num", GetSeqNum)
	_ = router.Run(":3307")
}

func preNodeReady(pre int) bool {
	if isFirst() {
		return true
	}
	host := getPodPrefix() + strconv.Itoa(pre)
	return isOpen(host, MysqlPort)
}

func isOpen(host string, port string) bool {

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	if conn != nil {
		defer conn.Close()
		return true
	}
	return false
}

func isFirst() bool {
	if getPodNum() == 0 {
		return true
	}
	return false
}
func GetSeqNum(context *gin.Context) {
	//dat, err := os.Open("/bitnami/mariadb/data/grastate.dat")
	dat, err := os.Open("./grastate.dat")
	if err != nil {
		context.JSON(500, nil)
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
		if c != nil {
			fmt.Println("出错:", c)
			break
		}
		if line != 4 {
			continue
		}
		str := string(s)
		num, err := strconv.Atoi(strings.TrimSpace(str[strings.IndexByte(str, ':')+1:]))
		if err != nil {
			context.JSON(500, nil)
		}
		context.JSON(200,
			Resp{"ok", num, 200})
		return

	}
	context.JSON(500, nil)

}

type Resp struct {
	Msg  string `json:"msg"`
	Data int    `json:"data"`
	Code int    `json:"code"`
}

func getPodPrefix() (name string) {
	name = PodName[:strings.LastIndex(PodName, "-")]
	return
}

func getPodNum() int {

	num, err := strconv.Atoi(PodName[strings.LastIndex(PodName, "-")+1:])
	if err != nil {
		panic(nil)
	}
	return num
}
