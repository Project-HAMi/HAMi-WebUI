export default {
  logDir: '/logs',
  replaceConsole: true,
  accessLogFormat:
    '[:localDate]^^A[:nginx-ip]^^A:method^^A:url^^AHTTP/:http-version^^A:status^^A:content-length^^Areferrer^^A:user-agent^^A:haodfglobal^^A:ip',
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
