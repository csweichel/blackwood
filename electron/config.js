const { app } = require("electron");
const fs = require("fs");
const path = require("path");

const configPath = path.join(app.getPath("userData"), "config.json");

const defaults = {
  url: "http://localhost:8090",
};

function load() {
  try {
    const data = JSON.parse(fs.readFileSync(configPath, "utf-8"));
    return { ...defaults, ...data };
  } catch {
    return { ...defaults };
  }
}

function save(config) {
  fs.mkdirSync(path.dirname(configPath), { recursive: true });
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
}

module.exports = { load, save };
