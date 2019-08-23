local http = require "http"
local json = require "json"
local mysql = require "mysql"
local redis = require "redis"

infolog(string.format("%s %s %s", request:GetIP(),request.Method,request.RequestURI))

-- response:Write(string.format("hello %s\n",request:GetIP()))
if request.URL.Path == "/api/ip" then
    local resp = http.get("http://httpbin.org/ip")
    local d = json.decode(resp.body)
    print(d)
    infolog(string.format("ip %s",d.origin))
    local hdr = response:Header()
    hdr:Set("content-type","application/json")
    response:Write(resp.body)
    return 
end

if request.URL.Path == "/api/user" then
    local c = mysql.new()
    local ok, err = c:connect({host="127.0.0.1",user="root",password="112233",database="game"})
    if not ok then
        error(err)
        return
    end

    local res, err = c:query("select * from `user` limit 3")
    if err ~= nil then
        error(err)
        return
    end
    for k,v in pairs(res) do
        print(k,v)
        for k1,v1 in pairs(v) do
            print(k1,v1)
        end
    end
    local hdr = response:Header()
    hdr:Set("content-type","application/json")
    response:Write(json.encode(res))
    c:close()
    return
end

if request.URL.Path == "/api/user2" then
    local c = redis.new({host="127.0.0.1"})
    local keys = c:Do("keys", "wallet*"):Val()
    for k, v in keys() do
        response:Write(string.format("%s %s\n", k, v))
    end
    c:Close()
end