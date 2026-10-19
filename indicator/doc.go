// Package indicator 提供与交易所无关的常见技术指标和市场结构批量计算函数。
//
// 所有序列结果均与输入等长，并通过 Series.Start 标识首个有效值下标。
// 本包只负责数值计算，不生成买卖信号，也不维护交易执行状态。
package indicator
