#!/usr/bin/env python
# coding=utf-8
#
# Copyright 2013 cinience
#


import tornado.httpserver
import tornado.ioloop
import tornado.options
import tornado.web
import os.path
import redis
import json
from tornado.options import define, options


def GetSettings():
    with open("sakerweb.conf") as config:
        return json.load(config)
        
config = GetSettings()
define("port", default=config["web-port"], help="run on the given port", type=int)
client = redis.Redis(config["saker-ip"], config["saker-port"])


class MainHandler(tornado.web.RequestHandler):
    def get(self):
        self.redirect("api")
        
class ApiHandler(tornado.web.RequestHandler):
    def get(self):
        self.redirect("api/index.html")

class InfoHandler(tornado.web.RequestHandler):
    def get(self):
        self.redirect("api/index.html")
        
class SVStatus(tornado.web.RequestHandler):
    def get(self):
        str = client.execute_command("exec", "svstatus")
        obj = json.loads(str)
        self.render("svinfo.html", items=obj)
        
class SVStop(tornado.web.RequestHandler):
    def get(self):
        serviceName = self.get_argument("servicename", None)
        client.execute_command("exec", "svstop", serviceName)        
        self.redirect("/sv")
        
class SVStart(tornado.web.RequestHandler):
    def get(self):
        serviceName = self.get_argument("servicename", None)
        client.execute_command("exec", "svstart", serviceName)
        self.redirect("/sv")
        
def main():
    app = tornado.web.Application(
        [
            (r"/", MainHandler),
            (r"/api", ApiHandler),
            (r"/info", InfoHandler),
            (r"/api/(.*)", tornado.web.StaticFileHandler, {"path":"api"}),
            (r"/sv", SVStatus),
            (r"/svstop", SVStop),
            (r"/svstart", SVStart),
            ],
        template_path=os.path.join(os.path.dirname(__file__), "templates"),
        static_path=os.path.join(os.path.dirname(__file__), "static"),
        )
    app.listen(options.port)
    tornado.ioloop.IOLoop.instance().start()


if __name__ == "__main__":
    main()
