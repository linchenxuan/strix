# 事件系统 (Event System)

`strix` 框架提供了一个轻量级的、基于内存的事件系统，位于 `event` 包中。它旨在实现应用程序内部不同组件之间的解耦，通过发布/订阅（Publish/Subscribe）模式，让模块间可以方便地进行通信，而无需直接相互依赖。

## 设计理念

`strix` 的事件系统遵循简单、直观的设计原则。它是一个**同步**的事件总线，这意味着当一个事件被发布时，`Publish` 函数会**阻塞**，直到所有订阅了该主题的订阅者（Subscriber）都执行完毕。

这种同步设计确保了事件处理的即时性和顺序性，非常适合那些需要确保事件被立即处理的场景。

**核心组件**:
-   **`Publisher`**: 事件的发布者和中心管理者。它持有所有主题（Topic）和它们的订阅者列表。
-   **`Topic`**: 一个具名的话题或频道。每个 `Topic` 对应一类特定的事件。
-   **`Subscriber`**: 事件的订阅者和处理器。它本质上是一个回调函数，当其订阅的 `Topic`上有新事件发布时，该函数就会被调用。

## 快速入门

以下是一个完整的使用示例，展示了如何创建主题、注册订阅者以及发布事件：

```go
package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/linchenxuan/strix/event" // 确保路径正确
)

// 1. 定义一个订阅者函数
// Subscriber 的类型是 func(param any)
func onUserLogin(param any) {
	userID, ok := param.(int)
	if !ok {
		fmt.Println("接收到无效的用户ID类型")
		return
	}
	fmt.Printf("处理用户登录事件：用户ID %d 正在登录...\n", userID)
	// 模拟一些耗时操作
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("用户ID %d 的登录事件处理完毕。\n", userID)
}

func main() {
	// 2. 创建一个 Publisher 实例
	// 在真实的 strix 应用中，您可能会从 Strix 核心结构体中获取它
	publisher := event.Publisher{}

	const userLoginTopic = "user_login"

	// 3. 创建一个新的 Topic
	// 必须先创建 Topic 才能订阅和发布
	// 第二个参数是发布超时，但当前同步实现下主要起占位作用
	err := publisher.NewTopic(userLoginTopic, 5*time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Println("主题 'user_login' 创建成功。")

	// 4. 为 Topic 注册一个或多个订阅者
	err = publisher.RegisterSubscriber(userLoginTopic, onUserLogin)
	if err != nil {
		panic(err)
	}
	fmt.Println("为 'user_login' 主题注册了 onUserLogin 订阅者。")

	// 可以注册多个订阅者
	err = publisher.RegisterSubscriber(userLoginTopic, func(param any) {
		fmt.Println("另一个订阅者也被触发了！")
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("为 'user_login' 主题注册了另一个匿名订阅者。")


	// 5. 发布事件
	// 传入主题名称和事件内容 (这里是一个用户ID)
	fmt.Println("\n发布一个登录事件，用户ID为 123...")
	err = publisher.Publish(userLoginTopic, 123)
	if err != nil {
		panic(err)
	}

	fmt.Println("\n事件发布完成。Publish 函数是同步阻塞的，此时所有订阅者都已执行完毕。")
}
```

## API 参考

### `Publisher`

`Publisher` 是与事件系统交互的主要入口。

| 方法                   | 描述                                                                          |
| :--------------------- | :---------------------------------------------------------------------------- |
| `NewTopic(name, timeout)` | 创建一个新的主题。必须先创建主题，然后才能对其进行订阅或发布。                |
| `RegisterSubscriber(name, sub)` | 为指定的主题注册一个订阅者（`Subscriber` 函数）。可以注册多个。               |
| `Publish(name, data)`    | 向指定主题发布一个事件，并传递数据 `data`。此方法会同步阻塞，直到所有订阅者返回。 |

### `Subscriber`

`Subscriber` 是一种函数类型，定义了事件处理器的签名。

```go
type Subscriber func(param any)
```
当您实现一个 `Subscriber` 时，需要注意对 `param any` 进行类型断言，以安全地获取您期望的数据。

### 默认主题

`strix` 框架内置了一些默认的事件主题，用于框架内部的通信。

-   `event.ReloadConfig`: 当配置被重新加载时，会发布此事件。您可以订阅此主题来动态响应配置变更。

## 总结

`strix` 的事件系统为应用内模块化和解耦提供了一个简单而强大的工具。通过其同步的发布/订阅机制，您可以构建出响应迅速、逻辑清晰的事件驱动型功能。虽然它是一个基于内存的简单实现，但它为构建复杂的应用内通信流程提供了坚实的基础。
