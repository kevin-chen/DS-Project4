package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"strings"

	"github.com/kataras/iris/v12"
)

/*
	Design Own Protocol on top of TCP:
	Create new item in array: msg = 'CREATE JSONData\n'
	Reading array: msg = 'READ\n'
	Reading item: msg = 'READ index\n'
	Update item: msg = 'UPDATE index JSONData\n'
	Delete item: msg = 'DELETE index\n'
*/

type Umbrella struct {
	Name         string `json:"name"`
	Price        int    `json:"price"`
	Brand        string `json:"brand"`
	Availability bool   `json:"availability"`
	Color        string `json:"color"`
}

type Umbrellas []Umbrella

var leaderAddress string
var backendList []string

/*
Purpose: helper function to check for errors
*/

func checkError(mes string, err error) {
	if err != nil {
		fmt.Println(mes, err)
		return
	}
}

/*
Input: None
Output: an array of Umbrella from the backend
Purpose: retrieves all the Umbrella array data from backend
Behavior: calls the READ action to the backend and return array response
*/
func retrieveAllData() Umbrellas {
	updateLeaderNode()
	conn, err := net.Dial("tcp", leaderAddress)
	checkError("Error creating connection", err)

	fmt.Fprintf(conn, "READ\n")
	checkError("Error reading connection response", err)

	decoder := json.NewDecoder(conn)
	var response Umbrellas
	responseErr := decoder.Decode(&response)
	checkError("Error decoding", responseErr)
	fmt.Println("Response for retrieveAllData: ", response)

	conn.Close()
	return response
}

/*
Input: index string representing the "id" of the object, also the index in the Umbrellas array
Output: Umbrella object from the backend
Purpose: retrieve a specific Umbrella object from backend
Behavior: calls the READ action with index parameter and returns the Umbrella object
*/
func retrieveDataFrom(index string) Umbrella {
	updateLeaderNode()
	conn, err := net.Dial("tcp", leaderAddress)
	checkError("Error creating connection", err)

	msg := "READ " + index + "\n"
	fmt.Fprintf(conn, msg)
	checkError("Error reading connection response", err)

	decoder := json.NewDecoder(conn)
	var response Umbrella
	responseErr := decoder.Decode(&response)
	checkError("Error decoding", responseErr)
	fmt.Println("Response for retrieveDataFrom index "+index+": ", response)

	conn.Close()
	return response
}

/*
Input: index string representing the "id" of the object to delete
Output: None
Purpose: delete object from the array in the backend
Behavior: calls the DELETE action with index parameter
*/
func deleteDataFrom(index string) {
	updateLeaderNode()
	conn, err := net.Dial("tcp", leaderAddress)
	checkError("Error creating connection", err)

	msg := "DELETE " + index + "\n"
	fmt.Fprintf(conn, msg)

	conn.Close()
}

/*
Input: Umbrella
Output: None
Purpose: delete object from the array in the backend
Behavior: calls the DELETE action with index parameter
*/
func createNewItem(newItem Umbrella) {
	updateLeaderNode()
	conn, err := net.Dial("tcp", leaderAddress)
	checkError("Error creating connection", err)

	// Convert the struct object to bytes
	byteItem, err := json.Marshal(newItem)
	checkError("Error converting new item to byte", err)

	// Convert bytes to string in order to pass message
	msg := "CREATE " + string(byteItem) + "\n"
	fmt.Fprintf(conn, msg)

	conn.Close()
}

/*
Input: index string representing the "id" of the object to update, and Umbrella object of updated item
Output: None
Purpose: update object with the "id" with a new Umbrella object
Behavior: calls the UPDATE action with index parameter and new Umbrella object
*/
func updateDataAt(index string, updateItem Umbrella) {
	updateLeaderNode()
	conn, err := net.Dial("tcp", leaderAddress)
	checkError("Error creating connection", err)

	byteItem, err := json.Marshal(updateItem)
	checkError("Error converting new item to byte", err)

	msg := "UPDATE " + index + " " + string(byteItem) + "\n"
	fmt.Fprintf(conn, msg)

	conn.Close()
}

func startBackends(backendList []string) {
	for _,backend := range backendList {
		go func(backend string) {
			conn, err := net.Dial("tcp", backend)
			checkError("Error creating connection to pass all clear", err)

			msg := "AllClear \n"
			fmt.Println("Sending AllClear to ", backend)
			conn.Write([]byte(msg))

			response := make([]byte, 128)
			conn.Read(response)
			fmt.Println("Response for AllClear:", string(response))

			conn.Close()
		}(backend)
	}
}

func updateLeaderNode() {
	for _,backend := range backendList {
		go func(backend string) {
			conn, err := net.Dial("tcp", backend)
			checkError("Error creating connection", err)

			msg := "CheckLeader \n"
			fmt.Println("Sending CheckLeader to ", backend)
			conn.Write([]byte(msg))

			response := make([]byte, 128)
			conn.Read(response)
			fmt.Println("Response for CheckLeader:", string(response))

			conn.Close()

			if string(response) == "Leader" {
				leaderAddress = backend
			}
		}(backend)
	}
}

func main() {
	app := iris.Default()

	httpPort := flag.String("listen", "8080", "http port number for frontend server to listen on")
	backendPorts := flag.String("backend", ":8090", "list of all backends available")
	flag.Parse()
	backendList = strings.Split(*backendPorts, ",")
	fmt.Println(backendList)
	startBackends(backendList)

	// Register HTML view engine
	app.RegisterView(iris.HTML("./webpages", ".html"))

	/*
		Purpose: Home Page, shows all inventory on root page by displaying the inventory array
		Method: GET
		Behavior: Shows home.html page and passes in inventory array
	*/
	app.Get("/", func(ctx iris.Context) {
		inventory := retrieveAllData()
		fmt.Println("Reading entire array:", inventory)
		ctx.ViewData("inventory", inventory)
		ctx.View("home.html")
	})

	/*
		Purpose: Create Page, shows the html form to create a new item
		Method: GET
		Behavior: Shows newItem.html page, which is a form
	*/
	app.Get("/new-item", func(ctx iris.Context) {
		ctx.View("newItem.html")
	})

	/*
		Purpose: Create new item, action for create form
		Method: POST
		Behavior: Reads the submitted html form data and creates new Umbrella item
		and appends to inventory array, then redirects to home page
	*/
	app.Post("/new-item", func(ctx iris.Context) {
		newItem := Umbrella{}
		err := ctx.ReadForm(&newItem)
		if err != nil {
			fmt.Println("Issue with Post New Item:", err)
			return
		}
		createNewItem(newItem)
		ctx.Redirect("/")
	})

	/*
		Purpose: Read Page, shows detail for a specific item
		Method: GET
		Behavior: Shows details.html page, which is a form that fills in the item details
		where one is able to edit and update
	*/
	app.Get("/details/{num}", func(ctx iris.Context) {
		indexStr := ctx.Params().Get("num")
		item := retrieveDataFrom(indexStr)
		ctx.ViewData("item", item)
		ctx.ViewData("num", indexStr)
		ctx.View("details.html")
	})

	/*
		Purpose: Update Operation, updates item in array
		Method: POST
		Behavior: After updating the details page, this action creates a new item with the
		updated item details and replaces the original item in the inventory array
	*/
	app.Post("/details/{num}", func(ctx iris.Context) {
		indexStr := ctx.Params().Get("num")

		newItem := Umbrella{}
		err := ctx.ReadForm(&newItem)
		checkError("Error reading form", err)

		fmt.Println("Updating Item " + indexStr)
		updateDataAt(indexStr, newItem)
		ctx.Redirect("/")
	})

	/*
		Purpose: Delete Operation, removes item in array at index
		Method: POST
		Behavior: After clicking delete on the details page, this action removes the
		selected item (referenced by index) from the inventory array
	*/
	app.Post("/delete/{num}", func(ctx iris.Context) {
		indexStr := ctx.Params().Get("num")
		deleteDataFrom(indexStr)
		ctx.Redirect("/")
	})

	app.Run(iris.Addr(":" + *httpPort))
}
