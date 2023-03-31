#1.假设这是我的业务代码，请你将下面这段代码保存为 app.py,启动一个 Web 服务器，当接收到 HTTP 请求时，返回 “你好 付品欣,这是我制作的第一个镜像!” 以及 HOSTNAME 环境变量。
from flask import Flask
import os
app = Flask(__name__)
app.run(debug=True)

@app.route('/')
def hello_world():
    return '你好 付品欣,这是我制作的第一个镜像! ' + os.getenv("HOSTNAME") + ''
