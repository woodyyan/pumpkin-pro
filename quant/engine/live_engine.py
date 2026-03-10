import time
import threading
from datetime import datetime, timedelta
import pandas as pd
from queue import Queue

from utils.logger import setup_logger
from broker.base_broker import BaseBroker
from data.live_feed import LiveFeed
# 引入现有的策略以保持复用
from indicators.technical_indicators import TechnicalIndicators
from strategy.trend_strategy import TrendStrategy

logger = setup_logger("live_engine")

class LiveEngine:
    """
    实盘/模拟盘调度引擎
    负责:
    1. 定时拉取行情
    2. 触发策略计算
    3. 调用券商网关执行下单
    """
    def __init__(self, broker: BaseBroker, symbol: str, strategy_name: str = "TrendStrategy"):
        self.broker = broker
        self.symbol = symbol
        self.strategy_name = strategy_name
        self.is_running = False
        self.feed = None
        self.thread = None
        
        # 简单防抖：记录今天是否已经交易过，避免1分钟内疯狂下单
        self.last_trade_date = None
        
    def start(self):
        if self.is_running:
            logger.warning("实盘引擎已在运行中...")
            return
            
        logger.info(f"=== 启动实盘引擎 ({self.strategy_name}) 标的: {self.symbol} ===")
        
        # 1. 连接券商
        if not self.broker.connect():
            logger.error("网关连接失败，引擎无法启动")
            return
            
        # 2. 初始化行情缓存 (拉取过去60天数据)
        start_dt = (datetime.now() - timedelta(days=90)).strftime("%Y-%m-%d")
        self.feed = LiveFeed(self.broker, self.symbol, start_dt)
        
        # 3. 启动事件循环线程
        self.is_running = True
        self.thread = threading.Thread(target=self._run_loop, daemon=True)
        self.thread.start()
        
    def stop(self):
        logger.info("=== 正在停止实盘引擎... ===")
        self.is_running = False
        if self.thread:
            self.thread.join(timeout=3.0)
        self.broker.disconnect()
        logger.info("实盘引擎已停止。")
        
    def _run_loop(self):
        """主事件循环"""
        logger.info("开始监听市场行情...")
        
        # 在真实环境中，这里应该是订阅 Websocket。
        # 为了演示和简化，我们用轮询 (Polling) 的方式，每隔3秒拉一次最新价
        while self.is_running:
            try:
                # 1. 拉取最新价格
                current_price = self.broker.get_realtime_price(self.symbol)
                if current_price is None:
                    time.sleep(3)
                    continue
                    
                # 2. 更新 K 线
                now = datetime.now()
                self.feed.update_with_tick(current_price, now)
                
                # 3. 触发策略计算
                self._evaluate_strategy()
                
            except Exception as e:
                logger.error(f"引擎主循环发生异常: {e}")
                
            time.sleep(5) # 每5秒评估一次（实盘频率可调）
            
    def _evaluate_strategy(self):
        """复用现有的静态回测策略进行一次单步计算"""
        df = self.feed.get_current_dataframe()
        if df is None or len(df) < 60:
            # 数据不够计算 MA60，跳过
            return
            
        # 动态计算指标 (复用你原来的代码)
        indicator_calc = TechnicalIndicators(df)
        data_with_indicators = indicator_calc.calculate_all_indicators()
        
        # 运行策略 (复用你原来的代码)
        strategy = TrendStrategy(data_with_indicators)
        data_with_signals = strategy.generate_signals()
        
        # 获取最新一天的信号
        latest_row = data_with_signals.iloc[-1]
        signal = latest_row['signal']
        today_date = latest_row['date'].strftime("%Y-%m-%d")
        
        if signal != 0:
            logger.info(f"[{today_date}] 策略触发信号! Signal={signal}, Price={latest_row['close']}")
            
            # 防抖：同一天不再重复开单
            if self.last_trade_date == today_date:
                logger.debug(f"今日 {today_date} 已执行过交易，忽略重复信号")
                return
                
            self._execute_signal(signal, latest_row['close'])
            self.last_trade_date = today_date
            
    def _execute_signal(self, signal: int, price: float):
        """根据信号发送订单"""
        direction = "BUY" if signal == 1 else "SELL"
        qty = 100 # 演示固定买卖 100 股
        
        # 简单的仓位检查
        positions = self.broker.get_positions()
        has_position = any(p['symbol'] == self.symbol for p in positions)
        
        if direction == "BUY" and has_position:
            logger.info(f"产生买入信号，但当前已持有 {self.symbol}，跳过开仓。")
            return
            
        if direction == "SELL" and not has_position:
            logger.info(f"产生卖出信号，但当前无持仓，跳过平仓。")
            return
            
        try:
            # 真正的下单动作
            logger.warning(f"🚀 准备执行 {direction} {qty} 股 {self.symbol}，参考价 {price}")
            order_id = self.broker.place_order(self.symbol, direction, price, qty, "MARKET")
            logger.info(f"✅ 订单提交成功，ID: {order_id}")
            
            # TODO: 可以在这里发微信通知
        except Exception as e:
            logger.error(f"❌ 下单失败: {e}")
