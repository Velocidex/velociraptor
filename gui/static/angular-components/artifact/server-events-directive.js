'use strict';

goog.module('grrUi.artifact.serverEventsDirective');

const ServerEventsController = function($scope, $uibModal, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  this.uibModal_ = $uibModal;
  this.grrApiService_ = grrApiService;

  this.artifacts = [];
  this.selectedArtifact = {};

  this.names = [];
  this.params = {};

  this.flowArguments = {};

  this.opened = false;
  this.reportParams;

  // Start off with the time rounded to the current day.
  this.selected_date;

  this.dateOptions = {
    formatYear: 'yy',
    startingDay: 1,
    dateDisabled: function(dateAndMode) {
      var timestamp_start = 0;
      var timestamp_end = 0;
      var utc;

      if (dateAndMode.mode == "day") {
        // This is a hack! popup date time has to use local
        // timezone. We therefore convert it to utc.
        utc = moment(dateAndMode.date).format("YYYY-MM-DD");
        timestamp_start = moment.utc(utc + "T00:00:00").unix();
        timestamp_end = moment.utc(utc + "T23:59:59").unix();
      } else if(dateAndMode.mode == "month") {
        utc = moment(dateAndMode.date).format("YYYY-MM-");
        timestamp_start = moment.utc(utc + "01T00:00:00").unix();
        timestamp_end = moment.utc(utc + "30T23:59:59").unix();
      } else {
        utc = moment(dateAndMode.date).format("YYYY-");
        timestamp_start = moment.utc(utc + "01-01T00:00:00").unix();
        timestamp_end = moment.utc(utc + "12-30T23:59:59").unix();
      };

      var timestamps = this.selectedArtifact.timestamps;
      if (angular.isDefined(timestamps.length)) {
        for (var i=0; i<timestamps.length; i++) {
          var ts = timestamps[i];
          if (ts >= timestamp_start && ts <= timestamp_end) {
            return false;
          }
        }
      }

      return true;
    }.bind(this),
  };

  this.scope_.$watch('controller.selected_date',
                     this.onDateChange.bind(this));
  this.GetArtifactList();
};

ServerEventsController.prototype.onDateChange = function() {
  if (!angular.isDefined(this.selected_date)) {
    return;
  }

  var timestamp = this.selected_date.getTime()/1000;
  this.reportParams = {
    "artifact": this.selectedArtifact.artifact,
    "type": "SERVER_EVENT",
    "start_time": timestamp,
    "end_time": timestamp+60*60*24,
  };
};

ServerEventsController.prototype.GetArtifactList = function() {
  var url = 'v1/ListAvailableEventResults';
  var params = {};
  return this.grrApiService_.post(url, params).then(
    function(response) {
      this.artifacts = response.data;
    }.bind(this));
};


ServerEventsController.prototype.openDatePicker = function() {
  this.opened = true;
};

ServerEventsController.prototype.selectArtifact = function(artifact) {
  this.selectedArtifact = artifact;
  this.selected_date = null;

  if (artifact.timestamps.length > 0) {
    var last_timestamp = artifact.timestamps[artifact.timestamps.length-1];
    this.selected_date = new Date(last_timestamp * 1000);
  }

  return false;
};

ServerEventsController.prototype.showHelp = function() {
    var self = this;
    self.modalInstance = self.uibModal_.open({
        templateUrl: '/static/angular-components/client/virtual-file-system/help.html',
        scope: self.scope_,
        size: "lg",
    });
  return false;
};


ServerEventsController.prototype.updateServerMonitoringTable = function() {
    var url = 'v1/GetServerMonitoringState';
    var self = this;

    this.error = "";
    this.grrApiService_.get(url).then(function(response) {
        self.flowArguments = response['data'];
        if (angular.isObject(self.flowArguments.artifacts)) {
            self.names = self.flowArguments.artifacts || [];
            self.params = {};
            var parameters = self.flowArguments.parameters.env || {};
            for (var i=0; i<parameters.length;i++) {
                var p = parameters[i];
                self.params[p["key"]] = p["value"];
            }
        }
        self.modalInstance = self.uibModal_.open({
            templateUrl: '/static/angular-components/artifact/add_server_monitoring.html',
            scope: self.scope_,
            size: "lg",
        });
    });
    return false;
};

ServerEventsController.prototype.saveServerArtifacts = function() {
    var self = this;

    // Update the names and the parameters.
    var env = [];
    for (var k in self.params) {
        if (self.params.hasOwnProperty(k)) {
            env.push({key: k, value: self.params[k]});
        }
    }

    self.flowArguments.artifacts = self.names;
    self.flowArguments.parameters = {env: env};

    var url = 'v1/SetServerMonitoringState';
    this.grrApiService_.post(
        url, self.flowArguments).then(function(response) {
        if (response.data.error) {
            this.error = response.data['error_message'];
        } else {
            this.modalInstance.close();
        }
    }.bind(this), function(error) {
        this.error = error;
    }.bind(this));
};



/**
 * Directive that displays artifact descriptor (artifact itself, processors and
 * source).
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.ServerEventsDirective = function() {
  return {
    scope: {
      "artifact": '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/artifact/server-events.html',
    controller: ServerEventsController,
    controllerAs: 'controller'
  };
};


exports.ServerEventsDirective.directive_name = 'grrServerEvents';
