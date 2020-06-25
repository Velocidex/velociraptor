'use strict';

goog.module('grrUi.utils.uploadDirective');


exports.UploadDirective = function($parse) {
    return {
        restrict: 'A',
        link: function(scope, element, attrs) {
            var model = $parse(attrs.grrUpload);
            var modelSetter = model.assign;

            element.bind('change', function() {
                scope.$apply(function() {
                    modelSetter(scope, element[0].files[0]);
                });
            });
        }
    };
};


exports.UploadDirective.directive_name = 'grrUpload';
