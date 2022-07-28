package rpc

import (
	_ "bufio"
	"log"
	"net/rpc"
)

// var (
// 	addr     = "127.0.0.1:" + strconv.Itoa(Port)
// 	request  = &core.Request{Name: Request}
// 	response = new(core.Response)
// )
//

func GetNFTsByIds(ids []int) error {
	client, err := rpc.Dial("tcp", "localhost:42586")
	if err != nil {
		log.Fatal(err)
	}

	var reply bool
	err = client.Call("Listener.GetLine", ids, &reply)
	if err != nil {
		log.Fatal(err)
	}

	return err
}
