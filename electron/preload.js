const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("blackwood", {
  getConfig: () => ipcRenderer.invoke("get-config"),
  saveConfig: (config) => ipcRenderer.invoke("save-config", config),
});
