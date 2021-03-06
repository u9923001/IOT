package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode"
	//"time"

	com "../common"
	"github.com/gorilla/websocket"
	"github.com/influxdata/influxdb/client/v2"
    
    "crypto/tls"
    "golang.org/x/crypto/acme/autocert"
    
)

var configFile = flag.String("c", "config.json", "config file")

//Sensor data struct
type Sensor = com.Sensor

//Print row number & error
func dbgErr(n string, p interface{}) {
	fmt.Printf("%s: ", n)
	fmt.Println(p)
}

//Create influxDB connection
func influxDBClient(u, p string) client.Client {
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://localhost:8086",
		Username: u,
		Password: p,
	})
	if err != nil {
		dbgErr("47", err)
		return nil
	}
	return c
}

//Write data to influxDB
func create7688Metrics(c client.Client, dataBase string, v Sensor, keypool *KeyPool) {

	verify := false
	key := keypool.Get(v.Id)
	if key != nil {
		verify = v.VerifySign(key)
	}

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  dataBase,
		Precision: "s",
	})

	if err != nil {
		dbgErr("60", err)
		return
	}

	ids := strconv.Itoa(int(v.Id))
	tags := map[string]string{
		"Id":        ids,
		"Device_id": v.Device_id,
	}
	fields := map[string]interface{}{
		"App":         v.App,
		"Device":      v.Device,
		"SiteName":    v.SiteName,
		"Longitude":   v.Longitude,
		"Latitude":    v.Latitude,
		"Speed_kmph":  v.Speed_kmph,
		"Timestamp":   v.Timestamp,
		"Temperature": v.Temperature,
		"Barometer":   v.Barometer,
		"Pm1":         v.Pm1,
		"Pm25":        v.Pm25,
		"Pm10":        v.Pm10,
		"Humidity":    v.Humidity,
		"Satellites":  v.Satellites,
		"Voltage":     v.Voltage,
		"Verified":    verify,
	}
	//udbtime, _ := time.Parse(time.RFC3339,strconv.Itoa(int(v.Timestamp)))
	pt, err := client.NewPoint("mobile", tags, fields, v.Timestamp)
	if err != nil {
		dbgErr("88", err)
		return
	}
	bp.AddPoint(pt)

	err = c.Write(bp)
	if err != nil {
		dbgErr("94", err)
		return
	}
}

// run Query on influxDB
func queryDB(clnt client.Client, dataBase string, cmd string) (res []client.Result, err error) {
	q := client.Query{
		Command:  cmd,
		Database: dataBase,
	}
	if response, err := clnt.Query(q); err == nil {
		if response.Error() != nil {
			return res, response.Error()
		}
		res = response.Results
	} else {
		return res, err
	}
	return res, nil
}

//Check string
func checkSD(v Sensor) (x bool) {
	x = false
	if !checkT(v.SiteName) {
		return x
	}

	if !checkR(v.Device_id) {
		return x
	}

	if !checkR(v.Device) {
		return x
	}

	if !checkR(v.App) {
		return x
	}
	x = true
	return x
}

func main() {
	flag.Parse()

	err, config := parseJSONConfig(*configFile)
	if err != nil {
		fmt.Println("load config file error", err)
		os.Exit(-1)
	}

	verbosity = config.Verb

	// save all endpoint's public key
	keypool := &KeyPool{
		endpoints: make(map[uint32]*Key),
		path:      config.KeyDir,
	}

	dbClient := influxDBClient(config.User, config.Pass)
	sess := NewSession()

	//定時抓資料
	lcache := NewLassCache()
	go GetLassData(sess, lcache, dbClient, config.DbName)

	myRouter := http.NewServeMux()

	//LASS感測器資料
	myRouter.HandleFunc("/socket", func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
		//ReadBufferSize:  1024,
		//WriteBufferSize: 1024,
		}
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		list := lcache.GetAll()
		fmt.Println("[ws]", len(list))
		for _, buf := range list {
			conn.WriteMessage(1, buf)
		}

		wsconn := sess.Add(conn)
		defer sess.Del(wsconn)

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			cmd := strings.SplitN(string(message), ",", 2)
			mlen := len(cmd)
			Vln(3, "[cmd]", mlen, cmd)
			if mlen != 2 {
				goto END
			}
			switch cmd[0] {
			case "0": //history
				res, err := queryDB(dbClient, config.DbName, "SELECT * FROM sensor WHERE Device_id = '"+cmd[1]+"' AND time > now() - 1d")
				if err != nil {
					wsconn.Send(1, []byte("0,[]"))
					goto END
				}
				b, err := json.Marshal(res)
				if err != nil {
					wsconn.Send(1, []byte("0,[]"))
					goto END
				}
				b = append([]byte("0,"), b...)
				wsconn.Send(messageType, b)

			case "idw": //IDW
				res, err := queryDB(dbClient, config.DbName, "SELECT Humidity,Latitude,Longitude,Pm25,Temperature FROM sensor WHERE time > now() - 5m")
				b, err := json.Marshal(res)
				if err != nil {
					wsconn.Send(1, []byte("idw,[]"))
					goto END
				}
				b = append([]byte("idw,"), b...)
				wsconn.Send(messageType, b)
            /*new adddddddddddddddddddddd*/
            case "mobile": //mobile id
				res, err := queryDB(dbClient, config.DbName, "SELECT * FROM mobile WHERE Device_id = '"+cmd[1]+"' AND time > now() - 10s")
				b, err := json.Marshal(res)
                fmt.Println("aaa")
				if err != nil {
					wsconn.Send(1, []byte("mobile,[]"))
					goto END
				}
				b = append([]byte("mobile,"), b...)
				wsconn.Send(messageType, b)
			}

		END:
		}
	})
    //Handle iot2040 get from db
	myRouter.HandleFunc("/Getsensor", func(w http.ResponseWriter, r *http.Request) {
        
        w.Header().Add("Access-Control-Allow-Origin", "*")

        // only POST
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
        
        type Dev_id struct {
            Device_id string `json:"Device_id,omitempty"`
        }
        
        v := Dev_id{}
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&v)
		if err != nil {
			dbgErr("104", err)
			return
		}
        
        res, err := queryDB(dbClient, config.DbName, "SELECT * FROM mobile WHERE Device_id = '"+v.Device_id+"' AND time > now() - 10s")
        //b, err := json.Marshal(res)
        if(err != nil){
            json.NewEncoder(w).Encode("")
        }else{
            json.NewEncoder(w).Encode(res)
        }
    })
	//Handle 7688 send to db
	myRouter.HandleFunc("/sensor", func(w http.ResponseWriter, r *http.Request) {

		// only POST
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		//Json decoder
		v := Sensor{}
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&v)
		if err != nil {
			dbgErr("104", err)
			return
		}
		//Check sensor data
		if checkSD(v) {
			go create7688Metrics(dbClient, config.DbName, v, keypool)
			Vln(4, "[data][new]", v)
		} else {
			Vln(4, "[data][err]", v)
		}
	})

	fsHandle := http.StripPrefix("/", http.FileServer(http.Dir("./static/")))
	myRouter.Handle("/", fsHandle)

	Vln(1, "bind @", config.LocalAddr)

	//--HTTP
	//http.ListenAndServe(config.LocalAddr, myRouter)

	//--HTTPS-1
	//http.ListenAndServeTLS(config.LocalAddr, "server.crt", "server.key", myRouter)

	//--HTTPS-2
    certManager := autocert.Manager{
        Prompt:     autocert.AcceptTOS,
        HostPolicy: autocert.HostWhitelist(config.DomainName),
        Cache:      autocert.DirCache("certs"),
    }
    htServer := &http.Server{
        Addr:    config.LocalAddr,
        Handler: myRouter,
        TLSConfig: &tls.Config{
            GetCertificate: certManager.GetCertificate,
        },
    }
    htServer.ListenAndServeTLS("", "")

}

func checkR(s string) (x bool) {
	if len(s) > 128 {
		return
	}
	last := false
	for _, c := range s {
		if c == '_' {
			if last {
				return
			}
			last = true
			continue
		}
		if c >= 'a' && c <= 'z' {
			last = false
			continue
		}
		if c >= 'A' && c <= 'Z' {
			last = false
			continue
		}
		if c >= '0' && c <= '9' {
			last = false
			continue
		}
		return
	}
	return true
}

func checkT(s string) (x bool) {
	if len(s) > 128 {
		return
	}
	last := false
	for _, c := range s {
		if c == '_' {
			if last {
				return
			}
			last = true
			continue
		}
		if c >= 'a' && c <= 'z' {
			last = false
			continue
		}
		if c >= 'A' && c <= 'Z' {
			last = false
			continue
		}
		if c >= '0' && c <= '9' {
			last = false
			continue
		}
		if unicode.In(c, unicode.Han) {
			last = false
			continue
		}
		return
	}
	return true
}
