#### Go语言单元测试框架

go语言的单元测试采用内置的测试框架,通过引入testing包以及go test来提供测试功能。

在源代码包目录内，所有以_test.go为后缀名的源文件被go test认定为测试文件，这些文件不包含在go build的代码构建中,而是单独通过 go test来编译，执行。


```bash
#-v是显示出详细的测试结果, -cover 显示出执行的测试用例的测试覆盖率。
go test -v -cover=true ./metrics/prometheus/prometheus_test.go 
```


#### 自动生成表格驱动的测试用例

在go语言中表格驱动测试非常常见。表格驱动的测试用例是在表格中预先定义好输入，期望的输出，和测试失败的描述信息，

然后循环表格调用被测试的方法，根据输入判断输出是否与期望输出一致，不一致时则测试失败, 返回错误的描述信息。

这种方法易于覆盖各种测试分支 ，测试逻辑代码没有冗余，开发人员只需要向表格添加新的测试数据即可。

对于适用于表格驱动测试的源码，我们采用开源工具gotests来自动生成测试用例。


开发人员只需要将不同的测试数据按照tests定义的结构写在//TODO:Add test cases下面，测试用例就完成了。


#### mock的使用实践

mock是单元测试中常用的一种测试手法，mock对象被定义，并能够替换掉真实的对象被测试的函数所调用。

而mock对象可以被开发人员很灵活的指定传入参数，调用次数，返回值和执行动作，来满足测试的各种情景假设。

那什么情况下需要使用mock呢?一般来说分这几种情况:

* 依赖的服务返回不确定的结果，如获取当前时间。
* 依赖的服务返回状态中有的难以重建或复现，比如模拟网络错误。
* 依赖的服务搭建环境代价高，速度慢，需要一定的成本，比如数据库，web服务
* 依赖的服务行为多变。

为了保证测试的轻量以及开发人员对测试数据的掌控，采用mock来斩断被测试代码中的依赖不失为一种好方法。

每种编程语言根据语言特点其所采用的mock实现有所不同。

在go语言中，mock一般通过两种方法来实现，一种是依赖注入，一种是通过interface,下面我们分别通过例子来说明这两种技术实践。


mock实现是通过go语言的interface，被mock的对象需要继承interface,并在interface中定义好被mock对象的方法。

mock对象通过实现interface的所有方法来表明自己实现了这个interface，这样mock对象的值就可以替换被mock对象的值。

对于mock对象我们可以自己定义实现，也可以通过工具实现。开源软件gomock3可以根据指定的interface自动生成mock对象， 并对mock对象自定义行为和返回结果，检查被调用次数，是一款非常好用的工具。

下面通过一个简单的示例来描述如何使用gomock工具。

1. 首先从github上获取gomock的相关源码包，并将其放在项目的vendor目录中。
   
```bash
go get github.com/golang/mock/gomock
go get github.com/golang/mock/mockgen

```
2. 将需要mock的方法放在interface中,使用mockgen命令指定接口实现mock接口,命令为:

```bash
mockgen -source {source_file}.go -destination {dest_file}.go
```