
function UUID_Example()
  saker.log(LOG_INFO, "This is UUID_Example")
  saker.log(LOG_INFO, "UUID : "..saker.uuid())
  return True
end

saker.register("UUID_Example", "UUID_Example", PROP_ONCE)