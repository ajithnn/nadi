package flow

import (
  "github.com/ajithnn/go-flow/scanner"
  "github.com/golang/glog"
  "encoding/json"
  "io/ioutil"
  "path"
  "strings"
  "time"
)

type Asset interface {
  Process(string, func())
}

type notImplemented struct{
}

func (n notImplemented) Process(filepath string,postProcess func()){
  defer postProcess()
  glog.V(2).Info("Type not found , unable to process",filepath)
  return
}

type Flow struct{
  scanPath string
  pipePath string
  whiteList string
  timeout float64
  scanTimeout time.Duration
  typeMap map[string]Asset
  getPrioritizedList func([]string)[]string
}

var pipeChannels = make(map[string](chan struct{}))
var channelTypes = make(map[string]string)
var processList = make(map[string]bool)
var filePathList []string
var FlowConfig Flow

func Trigger(config Flow) {
  FlowConfig = config
  readConfigAndCreateChannels()

  ch := make(chan string)
  w := scanner.FileScanner{FlowConfig.scanPath, FlowConfig.timeout, make(chan string), strings.Split(FlowConfig.whiteList,",")}
  for {
    go process(w.OutChannel, ch)
    go w.Scan()
    end := <-ch
    if end == "__DONE" {
      glog.V(2).Infof("Waiting for next scan")
      glog.Flush()
      time.Sleep(time.Second * FlowConfig.scanTimeout)
    }
  }
}

func process(c <-chan string, ch chan<- string) {
  for {
    filepath := <-c
    if filepath == "__EOF" {
      glog.V(2).Infof("End of current Scan will continue after wait.")
      pushByPriority(filePathList)
      ch <- "__DONE"
    } else if len(filepath) > 0 {
      filePathList = append(filePathList,filepath)
    }
  }
}

func pushByPriority(fileList []string){
  filePathList = FlowConfig.getPrioritizedList(fileList)
  filepath := ""
  for len(filePathList) > 0 {
    filepath,filePathList = filePathList[0], filePathList[1:]
    typeToProcess,typeName := getTypeFromFilePath(filepath)
    actualProcess(typeToProcess, typeName, filepath)
  }
}

func readConfigAndCreateChannels(){
  configFilePath := FlowConfig.pipePath
  var tempPipe interface{}
  commonChannel := make(chan struct{},1)
  configFile, _ := ioutil.ReadFile(configFilePath)
  json.Unmarshal(configFile, &tempPipe)
  curPipe := tempPipe.(map[string]interface{})
  for k,v := range curPipe{
    tempType := v.(map[string]interface{})
    glog.V(2).Infof("String %s Media values %f",k,tempType["capacity"].(float64))
    if tempType["type"] == "separate"{
      pipeChannels[k] = make(chan struct{},int(tempType["capacity"].(float64)))
    }else{
      pipeChannels[k] =commonChannel
    }
    channelTypes[k] = tempType["type"].(string)
  }
}

func getTypeFromFilePath(filepath string) (Asset,string) {
  dir := path.Dir(filepath)
  for k := range pipeChannels{
    if strings.Contains(strings.ToLower(dir),strings.ToLower(k)){
      return FlowConfig.typeMap[k],k
    }
  }
  return notImplemented{},""
}

func actualProcess(processType Asset,typeName string, filepath string) {
  if _,ok := processList[filepath]; !ok {
    select {
    case pipeChannels[typeName] <- struct{}{}:
      processList[filepath] = true
      go processType.Process(filepath,func(){
        delete(processList,filepath)
        <-pipeChannels[typeName]
        glog.V(2).Infof("Released channel and cleared file hold.")
      })
    default:
      glog.V(2).Infof("All channels blocked for type %s",typeName)
    }
  }
}
