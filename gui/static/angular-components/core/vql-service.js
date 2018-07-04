'use strict';

goog.module('grrUi.core.vqlService');
goog.module.declareLegacyNamespace();


exports.generateVQLResponse = function(response) {
  return JSON.parse(response.response);
}
