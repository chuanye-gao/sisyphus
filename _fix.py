import os, re 
BASE = r'D:\GithubRepositories\LongWay\sisyphus' 
 
# File 1: config.go 
path = os.path.join(BASE, 'pkg', 'config', 'config.go') 
with open(path, 'r', encoding='utf-8') as f: c = f.read() 
print('read config.go:', len(c), 'chars') 
old = 'Timeout          int     `yaml:\"timeout\"`            // sync req timeout' 
new = old + '\n\tStreamIdleTimeout int     `yaml:\"stream_idle_timeout\"` // idle timeout between stream chunks' 
c = c.replace(old, new) 
old2 = '\t\t\tTimeout:          120,' 
new2 = '\t\t\tTimeout:          120,\n\t\t\tStreamIdleTimeout: 60,' 
c = c.replace(old2, new2) 
with open(path, 'w', encoding='utf-8') as f: f.write(c) 
print('OK: config.go patched') 
