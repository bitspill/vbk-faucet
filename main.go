package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/bitspill/vbk-faucet/veriblock"
	"github.com/davecgh/go-spew/spew"
	"google.golang.org/grpc"
)

type FaucetTemplateContents struct {
	HasAlert         bool
	AlertColor       string
	AlertContents    template.HTML
	FaucetBalanceStr string
	FaucetVbkAddress string
}

func main() {
	conn, err := grpc.Dial("localhost:10501", grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client := veriblock.NewAdminClient(conn)

	tmpl := template.Must(template.ParseFiles("index.html"))

	rateLimitMap := map[string]struct{}{}

	go func() {
		for {
			select {
			case <-time.After(6 * time.Hour):
				rateLimitMap = map[string]struct{}{}
			}
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := FaucetTemplateContents{
			FaucetVbkAddress: "V3DmnZouE65km1SmNRrtenBtfBGEqf",
			FaucetBalanceStr: "0",
		}

		bal, err := client.GetInfo(context.Background(), &veriblock.GetInfoRequest{})
		if err != nil {
			fmt.Println(err)
			data.AlertColor = "red"
			data.AlertContents = "Unable to obtain faucet balance, please try again later."
			data.HasAlert = true
			_ = tmpl.Execute(w, data)
			return
		}

		data.FaucetBalanceStr = fmt.Sprintf("%0.8f", veriblock.AtomicToCoin(bal.DefaultAddress.Amount))

		addr := r.FormValue("addr")
		if addr != "" {

			if _, ok := rateLimitMap[r.RemoteAddr]; ok {
				data.AlertColor = "red"
				data.AlertContents = "Looks like you've already claimed some VBK recently, why not use what you've got for now and come back for more later if you still need it"
				data.HasAlert = true
				_ = tmpl.Execute(w, data)
				return
			}

			if len(addr) != 30 {
				data.AlertColor = "red"
				data.AlertContents = "Invalid VBK address"
				data.HasAlert = true
				_ = tmpl.Execute(w, data)
				return
			}

			addrBytes := veriblock.DecodeBase58(addr)
			if addrBytes != nil {
				amt := int64(10e8)
				min := bal.DefaultAddress.Amount / 100
				if min < amt {
					amt = min
				}
				scr := &veriblock.SendCoinsRequest{
					SourceAddress: addrBytes,
					Amounts: []*veriblock.Output{
						{
							Address: addrBytes,
							Amount:  amt,
						},
					},
				}

				reply, err := client.SendCoins(context.Background(), scr)
				if err != nil {
					fmt.Println("---------------\nError fulfilling request")
					spew.Dump(r.Form)
					spew.Dump(err)
					fmt.Println("----------------")
					data.AlertColor = "red"
					data.AlertContents = "Error sending VBK, please verify address is valid and/or try again later."
					data.HasAlert = true

				} else if reply.Success {
					data.HasAlert = true
					data.AlertContents = template.HTML(fmt.Sprintf("Sent %0.8f tVBK to %s<br />txid: %s",
						veriblock.AtomicToCoin(amt), addr, hex.EncodeToString(reply.TxIds[0])))
					data.AlertColor = "green"
					rateLimitMap[r.RemoteAddr] = struct{}{}
				}
			}
		}

		_ = tmpl.Execute(w, data)
	})

	_ = http.ListenAndServe("127.0.0.1:8086", nil)
}
