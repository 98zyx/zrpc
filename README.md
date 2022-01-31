# zrpc
一个简单易用的Go RPC框架

## 功能
* 负载均衡（一致性哈希，Round-Robin, 随机）
* 服务注册与发现
* 心跳功能
* 超时处理（调用超时，连接超时，处理超时）
* 支持TCP/HTTP网络协议
* 连接复用
* 同步/异步调用
* 支持gob/json序列化协议

## 简单用法
* 创建注册中心
``` Go
l, _ := net.Listen("tcp", ":9999")
registry.HandleHTTP()
```
* 创建服务端
``` Go
l, err := net.Listen("tcp", ":0")
server := service.NewServer(registryAddr, "tcp@"+l.Addr().String()) 
err = server.Register(&"service entity") // "service entity"指需注册的服务实体
server.Listen(l, 0) //服务器将自动向注册中心注册，并启动心跳服务
```
* 创建客户端
``` Go
xc := client.NewXClient(registryAddr, "strategy", nil, 0) // strategy指客户端指定的负载均衡策略，注册中心提供：ConsistentHash，RoundRobin，RandomSelect负载均衡策略，0表示对连接不做时间要求
```
* 调用服务
``` Go
err = xc.Call(serviceMethod, args, &reply, timeout) // serviceMethod指调用的服务，timeout指调用超时阈值
```

## 更改协议
用户在创建客户端时传入自定义的协议，更改序列化协议与最大调用时间
``` Go
type Option struct {
	MagicNumber int        // 标识该消息为zrpc请求
	CodecType   codec.Type // 表示请求的编码方式
	MaxCallTime time.Duration
}
```

## To do
* 增加协议的扩展性
* 设计协议头部
* 尝试使用零拷贝优化协议
* 取消协议头部的序列化/反序列化，直接从tcp流中解析头部
* 支持动态代理
* 支持protobuf序列化
* 支持注册中心消息总线集群
* RPC功能插件化
* 添加权重负载均衡策略
* 更为复杂细致的健康检测机制
* 支持添加自定义路由策略
* 心跳信号添加状态信息
* 根据节点健康状态动态调整权重

## License
Apache License, Version 2.0
