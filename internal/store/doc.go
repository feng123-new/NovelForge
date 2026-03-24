// Package store 提供基于文件系统的持久化存储。
//
// 设计边界保持克制：
//  1. store 负责文件读写、原子更新，以及"基于已存数据的查询/派生"。
//  2. store 可以回答"当前是什么状态"，也可以执行与持久化强绑定的局部更新。
//  3. store 不负责决定"下一步该怎么做"，流程推进策略应放在 orchestrator。
//
// 当前包内按职责分区：
//   - store.go: 底层文件 IO 与加锁
//   - progress.go: 进度主状态读写与 Flow/重写队列管理
//   - signals.go: 一次性信号文件（last_commit / last_review）
//   - outline.go: 大纲与分层大纲，以及基于大纲的边界查询
//   - 其他文件: 角色、摘要、时间线、关系、伏笔等领域数据
package store
