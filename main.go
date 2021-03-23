package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/grpc-ecosystem/grpc-gateway/codegenerator"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
	"protoc-gen-swagger/genswagger"
)

var (
	importPrefix               = flag.String("import_prefix", "", "prefix to be added to go package paths for imported proto files")
	file                       = flag.String("file", "-", "where to load data from")
	allowDeleteBody            = flag.Bool("allow_delete_body", false, "unless set, HTTP DELETE methods may not have a body")
	grpcAPIConfiguration       = flag.String("grpc_api_configuration", "", "path to gRPC API Configuration in YAML format")
	allowMerge                 = flag.Bool("allow_merge", false, "if set, generation one swagger file out of multiple protos")
	allowSave                  = flag.Bool("allow_save", false, "if set, save req.bin for debug")
	yapiUrl                    = flag.String("yapi_url", "", "yapi 地址")
	yapiSchema                 = flag.String("yapi_schema", "", "yapi 地址")
	yapiToken                  = flag.String("yapi_token", "", "yapi 项目 token")
	yapiMerge                  = flag.String("yapi_merge", "", "yapi merge [normal]-新增不覆盖 [good]-智能合并 [merge]-完全覆盖")
	wrapCode                   = flag.Bool("wrap_code", false, "if set, generation one swagger file out of multiple protos")
	mergeFileName              = flag.String("merge_file_name", "apidocs", "target swagger file name prefix after merge")
	useJSONNamesForFields      = flag.Bool("json_names_for_fields", false, "if it sets Field.GetJsonName() will be used for generating swagger definitions, otherwise Field.GetName() will be used")
	repeatedPathParamSeparator = flag.String("repeated_path_param_separator", "csv", "configures how repeated fields should be split. Allowed values are `csv`, `pipes`, `ssv` and `tsv`.")
	versionFlag                = flag.Bool("version", false, "print the current verison")
	allowRepeatedFieldsInBody  = flag.Bool("allow_repeated_fields_in_body", false, "allows to use repeated field in `body` and `response_body` field of `google.api.http` annotation option")
	includePackageInTags       = flag.Bool("include_package_in_tags", false, "if unset, the gRPC service name is added to the `Tags` field of each operation. if set and the `package` directive is shown in the proto file, the package name will be prepended to the service name")
	useFQNForSwaggerName       = flag.Bool("fqn_for_swagger_name", false, "if set, the object's swagger names will use the fully qualify name from the proto definition (ie my.package.MyMessage.MyInnerMessage")
)

// Variables set by goreleaser at build time
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	flag.Parse()
	defer glog.Flush()

	if *versionFlag {
		fmt.Printf("Version %v, commit %v, built at %v\n", version, commit, date)
		os.Exit(0)
	}

	var reg = genswagger.NewRegistry()

	glog.V(1).Info("Processing code generator request")
	f := os.Stdin
	if *file != "-" {
		var err error
		f, err = os.Open(*file)
		if err != nil {
			glog.Fatal(err)
		}
	}
	glog.V(1).Info("Parsing code generator request")
	req, err := codegenerator.ParseRequest(f)
	if err != nil {
		glog.Fatal(err)
	}

	glog.V(1).Info("Parsed code generator request")
	pkgMap := make(map[string]string)
	if req.Parameter != nil {
		err := parseReqParam(req.GetParameter(), flag.CommandLine, pkgMap)
		if err != nil {
			glog.Fatalf("Error parsing flags: %v, %s", err, *req.Parameter)
		}
	}

	reg.YAPISchema = *yapiSchema
	reg.YAPIUrl = *yapiUrl
	reg.YAPIToken = *yapiToken
	reg.YAPIMerge = *yapiMerge
	reg.SetWrapRespCode(*wrapCode)
	reg.SetPrefix(*importPrefix)
	reg.SetAllowDeleteBody(*allowDeleteBody)
	reg.SetAllowMerge(*allowMerge)
	reg.SetMergeFileName(*mergeFileName)
	reg.SetUseJSONNamesForFields(*useJSONNamesForFields)
	reg.SetAllowRepeatedFieldsInBody(*allowRepeatedFieldsInBody)
	reg.SetIncludePackageInTags(*includePackageInTags)
	reg.SetUseFQNForSwaggerName(*useFQNForSwaggerName)
	if err := reg.SetRepeatedPathParamSeparator(*repeatedPathParamSeparator); err != nil {
		emitError(err)
		return
	}
	for k, v := range pkgMap {
		reg.AddPkgMap(k, v)
	}

	if *grpcAPIConfiguration != "" {
		if err := reg.LoadGrpcAPIServiceFromYAML(*grpcAPIConfiguration); err != nil {
			emitError(err)
			return
		}
	}

	g := genswagger.New(reg)

	if err := genswagger.AddStreamError(reg); err != nil {
		emitError(err)
		return
	}

	if err := reg.Load(req); err != nil {
		emitError(err)
		return
	}

	var targets []*descriptor.File
	for _, target := range req.FileToGenerate {
		f, err := reg.LookupFile(target)
		if err != nil {
			glog.Fatal(err)
		}
		targets = append(targets, f)
	}

	out, err := g.Generate(targets)
	glog.V(1).Info("Processed code generator request")
	if err != nil {
		emitError(err)
		return
	}
	emitFiles(out)
	sendYAPI(out, reg)
}

// 发送到 yapi
func sendYAPI(out []*plugin.CodeGeneratorResponse_File, reg *genswagger.SRegistry) {
	var (
		schema = reg.YAPISchema
		addr   = schema + "://" + reg.YAPIUrl + "/api/open/import_data"
		token  = reg.YAPIToken
		merge  = reg.YAPIMerge
	)

	if len(addr) == 0 || len(token) == 0 || len(merge) == 0 {
		glog.Info("yapi_url:%s yapi_token:%s merge:%s", addr, token, merge)
		return
	}

	for _, item := range out {
		v := url.Values{}
		v.Set("type", "swagger")
		v.Set("token", token)
		v.Set("merge", merge)
		v.Set("json", *item.Content)

		client := &http.Client{}
		req, err := http.NewRequest("POST", addr, strings.NewReader(v.Encode()))
		if err != nil {
			glog.Error(err)
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			glog.Error(err)
		}
		defer resp.Body.Close()
		data, _ := ioutil.ReadAll(resp.Body)
		glog.Info(data)
	}
	glog.Info("upload yapi 成功")
}

func emitFiles(out []*plugin.CodeGeneratorResponse_File) {
	emitResp(&plugin.CodeGeneratorResponse{File: out})
}

func emitError(err error) {
	emitResp(&plugin.CodeGeneratorResponse{Error: proto.String(err.Error())})
}

func emitResp(resp *plugin.CodeGeneratorResponse) {
	buf, err := proto.Marshal(resp)
	if err != nil {
		glog.Fatal(err)
	}
	if _, err := os.Stdout.Write(buf); err != nil {
		glog.Fatal(err)
	}
}

// parseReqParam parses a CodeGeneratorRequest parameter and adds the
// extracted values to the given FlagSet and pkgMap. Returns a non-nil
// error if setting a flag failed.
func parseReqParam(param string, f *flag.FlagSet, pkgMap map[string]string) error {
	if param == "" {
		return nil
	}
	for _, p := range strings.Split(param, ",") {
		spec := strings.SplitN(p, "=", 2)
		if len(spec) == 1 {
			if spec[0] == "allow_delete_body" {
				err := f.Set(spec[0], "true")
				if err != nil {
					return fmt.Errorf("Cannot set flag %s: %v", p, err)
				}
				continue
			}
			if spec[0] == "allow_merge" {
				err := f.Set(spec[0], "true")
				if err != nil {
					return fmt.Errorf("Cannot set flag %s: %v", p, err)
				}
				continue
			}
			if spec[0] == "allow_save" {
				err := f.Set(spec[0], "true")
				if err != nil {
					return fmt.Errorf("Cannot set flag %s: %v", p, err)
				}
				continue
			}
			if spec[0] == "wrap_code" {
				err := f.Set(spec[0], "true")
				if err != nil {
					return fmt.Errorf("Cannot set flag %s: %v", p, err)
				}
				continue
			}
			if spec[0] == "allow_repeated_fields_in_body" {
				err := f.Set(spec[0], "true")
				if err != nil {
					return fmt.Errorf("Cannot set flag %s: %v", p, err)
				}
				continue
			}
			if spec[0] == "include_package_in_tags" {
				err := f.Set(spec[0], "true")
				if err != nil {
					return fmt.Errorf("Cannot set flag %s: %v", p, err)
				}
				continue
			}
			err := f.Set(spec[0], "")
			if err != nil {
				return fmt.Errorf("Cannot set flag %s: %v", p, err)
			}
			continue
		}
		name, value := spec[0], spec[1]
		if strings.HasPrefix(name, "M") {
			pkgMap[name[1:]] = value
			continue
		}
		if err := f.Set(name, value); err != nil {
			return fmt.Errorf("Cannot set flag %s: %v", p, err)
		}
	}
	return nil
}
