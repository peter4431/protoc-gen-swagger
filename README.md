# 流程
生成 swagger 文档，并能自动更新到 YAPI

## 使用样例

```shell
protoc 
    --proto_path="./proto" 
    --go_out=. 
    --go-grpc_out=. 
    --swagger_out=allow_merge,wrap_code,merge_file_name=api,yapi_schema=https,yapi_url=yapi.sagiteam.cn,yapi_token=9301a97eb4e9c3b3c398779939575abc81328d3e23d7545773df461ea04c28b1,yapi_merge=merge:. 
    proto/*.proto
```

## 支持特性

### 接口名和注释

```protobuf

// 基础服务
service Base{
  // 登录
  //
  // code 405-407
  rpc BaseLogin(BaseLoginReq) returns (LoginRsp) {
    option(google.api.http) = {post: "/v1/base/login", body:"*", response_body: "*"};
    option(extend.id) = {cmd: LOGIN};
  }
}

```

接口名为 `登录`,注释为 `code 405-407`,识别空行

### 接口 required

```protobuf
message LoginReq {
  string id = 1;      //`required` `pattern=[\w\d]{15}` 设备id
}
```
会在 YAPI 上标记为 `必须` 的字段，mock pattern 为 `[\w\d]{15}`

## 解释

swagger plugin 支持参数

- allow_merge bool 是否把 swagger 合并为一个，默认不合并，会是很多个 json 文件
- wrap_code   bool 是否把返回结果包装成 code,msg,body，为自定义协议
- merge_file_name string 如果合并的话，输出的 json 文件名
- yapi_url yapi 的地址，跟地址即可，会在后面加上 api/open/import_data 形成完整的地址
- yapi_schema yapi 的schema http/https
- yapi_token yapi 项目 token
- yapi_merge yapi merge [normal]-新增不覆盖 [good]-智能合并 [merge]-完全覆盖