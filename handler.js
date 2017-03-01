'use strict';

var child_process = require('child_process');

module.exports.main = (event, context, callback) => {
  var params = "" // JSON.stringify(event)
  var proc = child_process.spawn('./main', [], { stdio: 'inherit' });

  proc.on('close', (code) => {
    if(code !== 0) {
      return context.done(new Error("Process exited with non-zero status code"));
    }

    callback(null, "success");
  });
}
