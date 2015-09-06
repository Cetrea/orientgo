package orient_test

import (
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/istreamdata/orientgo"
	_ "github.com/istreamdata/orientgo/obinary"
)

const (
	orientVersion = "2.1.1"

	dbName = "default"
	dbUser = "admin"
	dbPass = "admin"

	srvUser = "root"
	srvPass = "root"
)

func init() {
	rand.Seed(time.Now().UnixNano())
	go func() {
		fmt.Println("pprof: ", http.ListenAndServe(":6060", nil))
	}()
}

func TestBuild(t *testing.T) {

}

func TestNewDB(t *testing.T) {
	_, closer := SpinOrient(t)
	defer closer()
}

func TestDBAuth(t *testing.T) {
	db, closer := SpinOrient(t)
	defer closer()
	if _, err := db.Auth(srvUser, srvPass); err != nil {
		t.Fatal("Connection to database failed")
	}
}

func TestDBAuthWrong(t *testing.T) {
	db, closer := SpinOrient(t)
	defer closer()
	if _, err := db.Auth(srvUser, srvPass+"pass"); err == nil {
		t.Fatal("auth error expected")
	}
}

func SpinOrientServer(t *testing.T) (string, func()) {
	const port = 2424

	dport_api := docker.Port("2424/tcp")
	dport_web := docker.Port("2480/tcp")
	binds := make(map[docker.Port][]docker.PortBinding)
	//	binds[dport_api] = []docker.PortBinding{docker.PortBinding{HostPort: fmt.Sprint(port)}}
	//	binds[dport_web] = []docker.PortBinding{docker.PortBinding{HostPort: fmt.Sprint(port + 1)}}

	cl, err := docker.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		t.Skip(err)
	}
	cont, err := cl.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			OpenStdin: true, Tty: true,
			ExposedPorts: map[docker.Port]struct{}{dport_api: struct{}{}, dport_web: struct{}{}},
			Image:        `dennwc/orientdb:` + orientVersion,
		}, HostConfig: &docker.HostConfig{
			PortBindings: binds,
		},
	})
	if err != nil {
		t.Skip(err)
	}

	rm := func() {
		cl.RemoveContainer(docker.RemoveContainerOptions{ID: cont.ID, Force: true})
	}

	if err := cl.StartContainer(cont.ID, &docker.HostConfig{PortBindings: binds}); err != nil {
		rm()
		t.Skip(err)
	}
	time.Sleep(time.Second * 5) // TODO: wait for input from container?

	info, err := cl.InspectContainer(cont.ID)
	if err != nil {
		rm()
		t.Skip(err)
	}

	return fmt.Sprintf("%s:%d", info.NetworkSettings.IPAddress, port), rm
}

func SpinOrient(t *testing.T) (*orient.Client, func()) {
	addr, rm := SpinOrientServer(t)
	cli, err := orient.Dial(addr)
	if err != nil {
		rm()
		t.Fatal(err)
	}
	return cli, func() {
		cli.Close()
		rm()
	}
}

func SpinOrientAndOpenDB(t *testing.T, graph bool) (*orient.Database, func()) {
	cli, closer := SpinOrient(t)
	tp := orient.DocumentDB
	if graph {
		tp = orient.GraphDB
	}
	db, err := cli.Open(dbName, tp, dbUser, dbPass)
	if err != nil {
		closer()
		t.Fatal(err)
	}
	return db, closer
}

var DocumentDBSeeds = []string{
	"CREATE CLASS Animal",
	"CREATE property Animal.name string",
	"CREATE property Animal.age integer",
	"CREATE CLASS Cat extends Animal",
	"CREATE property Cat.caretaker string",
	"INSERT INTO Cat (name, age, caretaker) VALUES ('Linus', 15, 'Michael'), ('Keiko', 10, 'Anna')",
}

func SeedDB(t *testing.T, db *orient.Database) {
	for _, seed := range DocumentDBSeeds {
		if _, err := db.SQLCommand(nil, seed); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSelect(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	docs, err := cli.SQLQuery(nil, nil, "SELECT FROM OUser")
	if err != nil {
		t.Fatal(err)
	} else if len(docs) != 3 {
		t.Error("wrong docs count")
	}
	//t.Logf("docs[%d]: %+v", len(docs), docs)
}

func TestSelectCommand(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	recs, err := cli.SQLCommand(nil, "SELECT FROM OUser")
	if err != nil {
		t.Fatal(err)
	} else if len(recs) != 3 {
		t.Error("wrong docs count")
	}
	//t.Logf("docs[%d]: %+v", len(recs), recs)
}

func TestSelectScript(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	recs, err := cli.ExecScript(nil, orient.LangSQL, "SELECT FROM OUser")
	if err != nil {
		t.Fatal(err)
	} else if len(recs) != 3 {
		t.Error("wrong docs count")
	}
	//t.Logf("docs[%d]: %+v", len(recs), recs)
}

func TestSelectScriptJS(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	recs, err := cli.ExecScript(nil, orient.LangJS, `var docs = db.query('SELECT FROM OUser'); docs`)
	if err != nil {
		t.Fatal(err)
	} else if len(recs) != 3 {
		t.Error("wrong docs count")
	}
	//t.Logf("docs[%d]: %+v", len(recs), recs)
}

func TestSelectSaveFunc(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	name := "tempFuncOne"
	code := `
	var param = one
	if (param != "some") {
		response.send(500, "err", "text/plain", "err" )
	}
	if (typeof(two) != "object") {
		response.send(500, "err2", "text/plain", "err2" )
	} else if (two.Name != "one") {
		response.send(500, "err3", "text/plain", "err3" )
	}
	var unused = "\\"
	var tbl = 'OUser'
	var docs = db.query("SELECT FROM "+tbl)
	return docs
	`
	if err := cli.CreateScriptFunc(orient.Function{
		Name: name, Code: code, Idemp: false,
		Lang: orient.LangJS, Params: []string{"one", "two"},
	}); err != nil {
		t.Fatal(err)
	}

	var fnc []struct {
		Name string
		Code string
	}
	if _, err := cli.SQLQuery(&fnc, nil, "SELECT FROM OFunction"); err != nil {
		t.Fatal(err)
	} else if len(fnc) != 1 {
		t.Fatal("wrong func count")
	} else if fnc[0].Name != name {
		t.Fatal("wrong func name")
	} else if fnc[0].Code != code {
		t.Fatal(fmt.Errorf("wrong func code:\n\n%s\nvs\n%s\n", fnc[0].Code, code))
	}

	recs, err := cli.CallScriptFunc(nil, name, "some", struct{ Name string }{"one"})
	if err != nil {
		t.Fatal(err)
	} else if len(recs) != 3 {
		t.Error("wrong docs count: ", len(recs), recs)
	}
	//t.Logf("docs[%d]: %+v", len(recs), recs)
}

func TestSelectSaveFunc2(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	name := "tempFuncTwo"
	code := `return {"params": [one, two]}`
	if err := cli.CreateScriptFunc(orient.Function{
		Name: name, Code: code, Idemp: false,
		Lang: orient.LangJS, Params: []string{"one", "two"},
	}); err != nil {
		t.Fatal(err)
	}

	var fnc []struct {
		Name string
		Code string
	}
	if _, err := cli.SQLQuery(&fnc, nil, "SELECT FROM OFunction"); err != nil {
		t.Fatal(err)
	} else if len(fnc) != 1 {
		t.Fatal("wrong func count")
	} else if fnc[0].Name != name {
		t.Fatal("wrong func name")
	} else if fnc[0].Code != code {
		t.Fatal(fmt.Errorf("wrong func code:\n\n%s\nvs\n%s\n", fnc[0].Code, code))
	}

	var res struct {
		Params []string
	}
	_, err := cli.CallScriptFunc(&res, name, "some", "one")
	if err != nil {
		t.Fatal(err)
	} else if len(res.Params) != 2 {
		t.Error("wrong result count")
	}
}

func TestSelectSaveFuncResult(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	name := "tempFuncOne"
	code := `return {"name":"ori","props":{"data":"ok","num":10,"custom":one}}`
	if err := cli.CreateScriptFunc(orient.Function{
		Name: name, Code: code, Idemp: false,
		Lang: orient.LangJS, Params: []string{"one"},
	}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Name  string
		Props map[string]interface{}
	}
	_, err := cli.CallScriptFunc(&result, name, "some")
	if err != nil {
		t.Fatal(err)
	} else if result.Name != "ori" {
		t.Fatal("wrong object name property")
	} else if len(result.Props) == 0 {
		t.Fatal("empty object props")
	}
	//t.Logf("doc: %+v", result)
}

func TestSelectSaveFuncResultJSON(t *testing.T) {
	cli, closer := SpinOrientAndOpenDB(t, false)
	defer closer()

	name := "tempFuncOne"
	code := `return {"name":"ori","props":{"data":"ok","num":10,"custom":one}}`
	if err := cli.CreateScriptFunc(orient.Function{
		Name: name, Code: code, Idemp: false,
		Lang: orient.LangJS, Params: []string{"one"},
	}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Name  string
		Props map[string]interface{}
	}
	_, err := cli.CallScriptFunc(&result, name, "some")
	if err != nil {
		t.Fatal(err)
	} else if result.Name != "ori" {
		t.Fatal("wrong object name property")
	} else if len(result.Props) == 0 {
		t.Fatal("empty object props")
	}
	//t.Logf("doc: %+v", result)
}