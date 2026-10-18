package model

// 结果落盘已改为每个任务独立的写入器（controller/SendAndWrite/sink.go），
// 不再使用全局文件句柄表 FileS：多任务并发时共享一个全局 map 会互相覆盖句柄，
// 且并发读写 map 会直接导致进程崩溃。
