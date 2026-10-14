-- GCRA, the same as ratelimit/memory. KEYS[1] holds the theoretical arrival time (TAT) in ms.
-- ARGV: interval ms, tolerance ms, now ms (-1 = Redis TIME, so every server shares one clock).
-- Returns {allowed 0|1, retry after ms, remaining}.
local interval = tonumber(ARGV[1])
local tolerance = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
if now < 0 then
  local t = redis.call('TIME')
  now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
end

local tat = tonumber(redis.call('GET', KEYS[1]))
if tat == nil or tat < now then
  tat = now
end

local allow_at = tat - tolerance
if now < allow_at then
  return {0, allow_at - now, 0}
end

local next_tat = tat + interval
-- Expires once fully refilled, plus 1 s.
redis.call('SET', KEYS[1], next_tat, 'PX', next_tat - now + 1000)
return {1, 0, math.floor((now + tolerance + interval - next_tat) / interval)}
