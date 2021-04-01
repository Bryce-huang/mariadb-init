package main

import (
	"bufio"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	GrastatePath  = "/bitnami/mariadb/data/grastate.dat"
	MysqlPort     = "3306"
	ConfigMapName = "init-mariadb-config"
)

var PodName = os.Getenv("MY_POD_NAME")
var REPLICAS = os.Getenv("REPLICAS")
var MariadbGaleraClusterAddress = os.Getenv("MARIADB_ADDRESS")
var signal = make(chan int)
var namespace = os.Getenv("NS")
var ClusterAddress = "mariadb-galera.default.svc.cluster.local"
var K8sConfig *rest.Config
var client *kubernetes.Clientset

func init() {
	var err error
	if K8sConfig, err = rest.InClusterConfig(); err != nil {
		K8sConfig, err = clientcmd.BuildConfigFromFlags("", "./config")
		if err != nil {
			log.Println("Get k8sConfig fail: ", err)
			panic(err)
		}

	}
	client, err = kubernetes.NewForConfig(K8sConfig)
	if err != nil {
		log.Println("Get k8sClient fail: ", err)
		panic(err)
	}

}

func main() {

	go task()
	<-signal
	log.Println("初始化退出。。。")
}

func statusReportAndGetAllStatus(node string, num string) []SeqNum {

	cmApi := client.CoreV1().ConfigMaps(namespace)
	// 第一次删除所有状态

	configMap, err := cmApi.Get(ConfigMapName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		fmt.Printf("Pod %s in namespace %s not found\n", "init-mariadb-config", namespace)

		r := &v1.ConfigMap{}
		r.Kind = "ConfigMap"
		r.APIVersion = "v1"
		r.Name = "init-mariadb-config"
		r.Labels = map[string]string{"int": "mariadb-galera", "app": "mariadb-galera"}
		r.Data = nil
		configMap, err = cmApi.Create(r)
		if err != nil {
			panic(err)
		}
	}

	fmt.Printf("origin: %v \n", configMap.Data)
	configMap.Data = map[string]string{
		node: num,
	}
	configMap, err = cmApi.Update(configMap)

	for err != nil {
		log.Println("configmap update failed, after 2 second retry")
		time.Sleep(time.Second * 2)
		configMap, err = cmApi.Update(configMap)
	}
	half, _ := strconv.Atoi(REPLICAS)
	// 不能以半数启动
	//half = half/2 + 1
	count := 0

	for {
		if checkClusterExits() {
			log.Print("集群已存在，可以启动")
			signal <- 0
			time.Sleep(time.Hour)
			return nil

		}
		configMap, err := cmApi.Get(ConfigMapName, metav1.GetOptions{})
		if err != nil {
			panic("can't read configmap from k8s")
		}

		if v, ok := configMap.Data[node]; !ok || v != num {
			configMap.Data[node] = num
			configMap, _ = cmApi.Update(configMap)
			log.Printf("configmap不存在数据，更新%v \n", v)
		}

		log.Printf("检查configMap：%d次，%v \n", count, configMap.Data)
		maps := configMap.Data

		log.Printf("map size is %d,half is %d \n", len(maps), half)
		if len(maps) == half {
			var res []SeqNum
			for s := range maps {
				node, _ := strconv.Atoi(s)
				num, _ := strconv.Atoi(maps[s])
				res = append(res, SeqNum{node, num})
			}
			log.Printf("发现seqnum slice： %v", res)
			return res
		}
		count++
		if count > 49 {
			panic("can't start after 50 times retry")
		}
		time.Sleep(time.Second * 3)
	}

	return nil
}

func task() {

	//  检查是否存在 grastate文件
	// 不存在 .检查本pod是不是id 为0，直接启动，不是的话 检查上一个node节点是否活跃，否则等待。直到上一个节点活跃
	if checkFileExits() {
		return
	}

	// 存在
	// 判断集群是否存在  存在集群，直接启动本机
	if checkClusterExits() {
		signal <- 0
		return
	}

	// 获取所有seqnum，确保所有节点都至少通信过一次，检查是否所有的都是一样的seqnum，
	seqNums := statusReportAndGetAllStatus(strconv.Itoa(getPodNum()), strconv.Itoa(getNum()))
	startIfReady(seqNums)
}

func startIfReady(seqNums []SeqNum) {
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
				time.Sleep(time.Second * 5)
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
		if preNodeReady(pre) {
			signal <- 0
			return
		}
		time.Sleep(time.Second * 5)
	}
}

func checkFileExits() bool {
	file, err := os.Open(GrastatePath)

	if os.IsNotExist(err) {
		log.Println("文件不存在,按照顺序启动。。。")
		if isFirst() {
			signal <- 0
			return true
		} else {
			count := 0
			for {
				count++
				log.Println("已重试", count, "次")
				if preNodeReady(getPodNum() - 1) {
					signal <- 0
					return true
				}
				time.Sleep(time.Second * 5)
			}
		}
	}

	if err != nil {
		panic(err)
	}
	defer file.Close()
	return false
}

func checkClusterExits() bool {

	for i := 0; i < 3; i++ {
		time.Sleep(5 * time.Second)
		if isOpen(ClusterAddress, MysqlPort) {
			signal <- 0
			return true
		}
	}
	log.Println("集群不存在。。。")
	return false
}

func meIsMax(seqNums []SeqNum) bool {
	return seqNums[0].node == getPodNum()

}

func getPreNum(seqNums []SeqNum) int {
	node := getPodNum()
	for i, v := range seqNums {
		if v.node == node {
			return seqNums[i-1].node
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

func preNodeReady(pre int) bool {

	host := getPodPrefix() + "-" + strconv.Itoa(pre) + "." + MariadbGaleraClusterAddress

	return isOpen(host, MysqlPort)
}

func isOpen(host string, port string) bool {
	url := net.JoinHostPort(host, port)
	log.Println("尝试连接地址：", url)
	conn, err := net.DialTimeout("tcp", url, 10*time.Second)
	if err != nil {
		log.Printf("链接失败: %s:%s,err:%v\n", host, port, err)
		return false
	}
	defer conn.Close()
	if conn != nil {
		log.Printf("链接成功: %s:%s\n", host, port)
		return true
	}
	log.Printf("链接失败: %s:%s\n", host, port)

	return false
}

func isFirst() bool {
	if getPodNum() == 0 {
		return true
	}
	return false
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
func getNum() int {

	file, err := os.Open(GrastatePath)
	//file, err := os.Open("./grastate.dat")
	if os.IsNotExist(err) {
		return -1
	}
	defer file.Close()
	br := bufio.NewReader(file)
	line := 0
	for {
		s, _, c := br.ReadLine()

		line++
		if c == io.EOF {
			break
		}
		if c != nil {
			log.Println("出错:", c)
			break
		}
		if line != 4 {
			continue
		}
		str := string(s)
		num, err := strconv.Atoi(strings.TrimSpace(str[strings.IndexByte(str, ':')+1:]))
		if err != nil {
			panic(err)
		}
		return num
	}
	return -1
}
