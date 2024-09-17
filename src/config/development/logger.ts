export default {
  logDir: '/logs',
  replaceConsole: true,
  accessLogFormat:
    '{ts::localDate,nginx_ip::nginx-ip,method::method,url::url,http-version::http-version,status::status,content-length::content-length,user-agent::user-agent,ip::ip}',
  appenders: {
    auditRecord: {
      type: 'dateFile',
      filename: 'auditRecord',
      pattern: 'yyyy-MM-dd.log',
      alwaysIncludePattern: true,
      fileNameSep: '-',
      level: 'INFO',
    },
  },
};
