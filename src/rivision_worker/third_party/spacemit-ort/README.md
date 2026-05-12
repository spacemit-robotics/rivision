## 目录

~~~ r
./
├── bin # 预编译可执行文件
├── doc # 版本发布与使用说明
├── include # 头文件依赖
│   ├── cpu_provider_factory.h
│   ├── provider_options.h
│   ├── onnxruntime*.h
│   └── spacemit_ort_env*.h
├── lib # 预编译库文件
├── python # python库
├── samples # demo源码
└── scripts # 编译与执行脚本
~~~

## QUICK_START
#### 编译
* riscv64
~~~ bash
# 声明RV GCC路径
export RISCV_ROOT_PATH={...}/spacemit-toolchain-linux-glibc-x86_64-v1.1.2
bash scripts/build_samples_riscv64.sh
~~~
* x86
~~~ bash
# 需要当前发布包为x86版本
bash scripts/build_samples_local.sh
~~~

#### 运行

* QEMU执行
~~~ bash
# 执行简单demo
# wget https://archive.spacemit.com/spacemit-ai/qemu/jdsk-qemu-v10.0.2.tar.gz
RV_QEMU_CMD={...}/bin/qemu-riscv64 -L {...}/spacemit-toolchain-linux-glibc-x86_64-v1.1.2/sysroot -cpu max,vlen=256,elen=64,vext_spec=v1.0
${RV_QEMU_CMD} run_demo resnet18 resnet18.onnx
~~~

~~~ bash
# 执行imagenet精度测试
${RV_QEMU_CMD} imagenet_test resnet18 imagenet_test_data/img_list.txt 4
~~~

* 在RV板上执行
~~~ bash
run_demo resnet18 resnet18.onnx
~~~

#### demo参数说明
* run_demo
~~~ r
# 最少只需2个参数
[net_name=str] # demo模型的名字
[net_param_path=str] # demo模型的onnx文件路径
[profile_prefix=str] # 未指定或指定未None则不开启profile，否则以此前缀开启profile并生成文件
[img_file_path=str] # 未指定或指定未None则不读取输入文件，否则读取该路径的文件
[num_threads=int] # 同时设置intra与inter的线程数为该值，默认为1
[loop_count=int] # >1时将统计平均耗时
[mean_value=str] # 以 , 为分割指定当前输入的mean，例如 128,128,128
[std_value=str] # 以 , 为分割指定当前输入的std，同上
[dyn_shape=str] # 以 ; 为分割指定多个输入的shape, 以 , 为分割指定某个shape具体的dim value, 例如"1,3,224,224;1,3,32,32"
# 可以不指定dyn_shape, 或不全指定，所有未被解析的dyn_shape的各维度值将被默认设为1
~~~
* imagenet_test
~~~ r
# 最少只需4个参数
[net_name=str] # demo模型的名字
[net_param_path=str] # demo模型的onnx文件路径
[img_file_list_path=str] # imagenet测试集的图像列表与label文件
[num_threads=int] # 同时设置intra与inter的线程数为该值，默认为1
[mean_value=str] # 以 , 为分割指定当前输入的mean，例如 128,128,128
[std_value=str] # 以 , 为分割指定当前输入的std，同上
~~~

## Code

#### C&C++
~~~ C
#include <onnxruntime_cxx_api.h>
#include "spacemit_ort_env.h"

Ort::Env env(ORT_LOGGING_LEVEL_WARNING, "ort-demo");
Ort::SessionOptions session_options;
std::unordered_map<std::string, std::string> provider_options;

// 分号间隔，数量等于intra_op_num_thread - 1，即不对主线程绑核，主线程需要手动绑核
//session_options.AddConfigEntry(kOrtSessionOptionsConfigIntraOpThreadAffinities, "1-4;1-4;1-4");
// 可禁止模型运行结束时线程缓存的释放, 在环境内只有少量模型运行时，可有一定加速效果
//session_options.AddConfigEntry(kOrtSessionOptionsAllowTLSCacheCleanup, "0");
// 以下为ep的配置
// provider_options["SPACEMIT_EP_DISABLE_OP_TYPE_FILTER"] = "OPA;OPB;OPC"; 禁止EP推理某些OP类型, node.op
// provider_options["SPACEMIT_EP_DISABLE_OP_NAME_FILTER"] = "OPA;OPB;OPC"; 禁止EP推理某些命名的OP, node.name
// provider_options["SPACEMIT_EP_DISABLE_FLOAT16_EPILOGUE"] = "1"; 禁止使用近似后处理
// provider_options["SPACEMIT_EP_DUMP_SUBGRAPHS"] = "1"; 导出ep编译子图，在执行程序的目录下，以SpaceMITExecutionProvider_SpineSubgraph_为前缀的ONNX模型
// provider_options["SPACEMIT_EP_DEBUG_PROFILE"] = "demo"; 导出ep执行profile, 以传入字符串为前缀的json
// provider_options["SPACEMIT_EP_DUMP_TENSORS"] = "demo"; 导出ep执行时的每层结果, 以传入字符串为文件夹, release版本没有这个功能
SessionOptionsSpaceMITEnvInit(session_options, provider_options); // 可选加载SpaceMIT环境初始化加载专属EP
Ort::Session session(env, net_param_path, session_options);

// ...后续与公版ORT一致
~~~

#### Python
~~~ python
# 使用whl包安装
# pip install onnxruntime*.whl
# pip install spacemit_ort-*.whl
# 如果遇到警告则加上--break-system-packages
# whl包剥离了依赖库的自动安装, 需要自行安装numpy

import onnxruntime as ort
import numpy as np
import spacemit_ort

eps = ort.get_available_providers() #
net_param_path = "resnet18.q.onnx"

# 带ep的session
session = ort.InferenceSession(net_param_path, providers=["SpaceMITExecutionProvider"])
# 不带ep的session
ref_session = ort.InferenceSession(net_param_path, providers=["CPUExecutionProvider"])

input_tensor = np.ones((1, 3, 224, 224), dtype=np.float32)
outputs = session.run(None, {"input.1": input_tensor})
ref_outputs = ref_session.run(None, {"input.1": input_tensor})

# outputs与ref_outputs的误差一般在1e-5以内
~~~