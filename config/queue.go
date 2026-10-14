package config

func (c *Config) validateQueue() error {
	q := c.Queue
	if q.Token != "" && (q.User != "" || q.Password != "") {
		return &InvalidSettingError{
			Key:    "queue.token",
			Reason: "set queue.token for a server without accounts, or queue.user and queue.password for one account, not both",
		}
	}
	if (q.User == "") != (q.Password == "") {
		return &MissingSettingError{Keys: "queue.user, queue.password", Reason: "set both or neither"}
	}
	return nil
}
