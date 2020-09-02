'use strict';

goog.module('grrUi.artifact.clientEventDirective');

const ClientEventController = function(
    $scope, $uibModal, grrApiService, grrRoutingService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.uibModal_ = $uibModal;
    this.grrApiService_ = grrApiService;
    this.grrRoutingService_ = grrRoutingService;

    this.artifacts = [];
    this.selectedArtifact = {};

    this.label = {};

    this.current_table = {};
    this.state = this._protobufToState({});

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
            for (var i=0; i<timestamps.length; i++) {
                var ts = timestamps[i];
                if (ts >= timestamp_start && ts <= timestamp_end) {
                    return false;
                }
            }

            return true;
        }.bind(this),
    };

    this.scope_.$watch('controller.label.label', this.onLabelChange.bind(this));
    this.scope_.$watch('controller.selected_date', this.onDateChange.bind(this));

    this.clientId;
    this.grrRoutingService_.uiOnParamsChanged(this.scope_, 'clientId',
                                              this.onClientIdChange_.bind(this));
    this.uiTraits = {};
    this.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
        this.uiTraits = response.data['interface_traits'];
    }.bind(this), function(error) {
        if (error['status'] == 403) {
            this.error = 'Authentication Error';
        } else {
            this.error = error['statusText'] || ('Error');
        }
    }.bind(this));
};


ClientEventController.prototype.onClientIdChange_ = function(clientId) {
  if (angular.isDefined(clientId)) {
    this.clientId = clientId;
  }

  this.GetArtifactList();
};


ClientEventController.prototype.onDateChange = function() {
  if (!angular.isDefined(this.selected_date)) {
    return;
  }

  var timestamp = this.selected_date.getTime()/1000;
  this.reportParams = {
    "artifact": this.selectedArtifact.artifact,
    "client_id": this.clientId,
    "type": "CLIENT_EVENT",
    "start_time": timestamp,
    "end_time": timestamp+60*60*24,
  };
};

ClientEventController.prototype.GetArtifactList = function() {
    if (angular.isString(this.clientId)) {
        var url = 'v1/ListAvailableEventResults';
        var params = {"client_id": this.clientId};
        return this.grrApiService_.post(url, params).then(
            function(response) {
                // Hide system events.
                var filter = new RegExp("System.");
                var available_result = [];
                for (var i=0; i<response.data.logs.length; i++) {
                    var artifact = response.data.logs[i];
                    if (!angular.isObject(artifact)) {
                        continue;
                    }
                    var name = artifact.artifact;
                    if (angular.isString(name) && !name.match(filter)) {
                        available_result.push(artifact);
                    }
                }

                this.artifacts = available_result;
            }.bind(this));
    };
};


ClientEventController.prototype.openDatePicker = function() {
  this.opened = true;
};

ClientEventController.prototype.selectArtifact = function(artifact) {
  this.selectedArtifact = artifact;
  this.selected_date = null;

  if (artifact.timestamps.length > 0) {
    var last_timestamp = artifact.timestamps[artifact.timestamps.length-1];
    this.selected_date = new Date(last_timestamp * 1000);
  }

  return false;
};

ClientEventController.prototype.showHelp = function() {
    var self = this;
    self.modalInstance = self.uibModal_.open({
        templateUrl: window.base_path+'/static/angular-components/client/virtual-file-system/help.html',
        scope: self.scope_,
        size: "lg",
    });
  return false;
};

ClientEventController.prototype.showClientMonitoringTables = function() {
    var self = this;
    var url = 'v1/GetClientMonitoringState';

    this.error = "";
    this.grrApiService_.get(url).then(function(response) {
        self.state = response['data'];

        var modalScope = self.scope_.$new();
        var modalInstance = self.uibModal_.open({
            template: '<grr-inspect-json json="json" '+
                'on-resolve="resolve()" title="Raw Client Monitoring Tables"/>',
            scope: modalScope,
            windowClass: 'wide-modal high-modal',
            size: 'lg'
        });

        modalScope["json"] = JSON.stringify(self.state, null, 2);
        modalScope["resolve"] = modalInstance.close;
    });

    return false;
};

// When the label changes, we need to switch the current table.
ClientEventController.prototype.onLabelChange = function() {
    if (!angular.isObject(this.state)) {
        return;
    }

    var label = this.label.label;

    // Switch to primary table
    if (label == "All") {
        this.current_table = this.state.artifacts;
        return;
    }

    // Do we have an existing table?
    for(var i=0; i<this.state.label_events.length;i++) {
        var table = this.state.label_events[i];

        if (table.label == label) {
            this.current_table = table.artifacts;
            return;
        }
    };

    // Not found, make a new table.
    this.current_table = {
        artifacts: [],
        parameters: {},
    };
    this.state.label_events.push({label:label, artifacts: this.current_table});
};

ClientEventController.prototype.updateClientMonitoringTable = function() {
    var self = this;
    var url = 'v1/GetClientMonitoringState';

    this.error = "";
    this.grrApiService_.get(url).then(function(response) {
        self.state = response['data'];

        // Convert protobuf to GUI state
        self._protobufToState(self.state);

        // Initial table will be set to primary table
        self.current_table = self.state.artifacts || {};
        self.modalInstance = self.uibModal_.open({
            templateUrl: window.base_path+'/static/angular-components/artifact/add_client_monitoring.html',
            scope: self.scope_,
            size: "lg",
        });
    });

    return false;
};

// Convert internal GUI event table state to ClientEventTable protobuf.
ClientEventController.prototype._stateToProtobuf = function(state) {
    var converter = function(table) {
        // Update the names and the parameters.
        var env = [];
        for (var k in table.parameters) {
            if (table.parameters.hasOwnProperty(k)) {
                env.push({key: k, value: table.parameters[k]});
            }
        }

        table.parameters = {env: env};
    };

    // Convert primary table.
    converter(state.artifacts);

    // Now convert each label table;
    for(var i=0; i<state.label_events.length; i++) {
        converter(state.label_events[i].artifacts);
    }
};

// Convert ClientEventTable protobuf to internal GUI event table state
ClientEventController.prototype._protobufToState = function(state) {
    var converter = function(table) {
        if (!angular.isArray(table.artifacts)) {
            table.artifacts = [];
        }

        if (!angular.isObject(table.parameters)) {
            table.parameters = {};
        }

        var state_parameters = {};
        var parameters = table.parameters.env || [];
        for (var i=0; i<parameters.length;i++) {
            var p = parameters[i];
            state_parameters[p["key"]] = p["value"];
        }

        table.parameters = state_parameters;
    };

    if(!angular.isObject(state.artifacts)) {
        state.artifacts = {};
    };

    // Convert primary table.
    converter(state.artifacts);

    if(!angular.isArray(state.label_events)) {
        state.label_events = [];
    };

    // Now convert each label table;
    for(var i=0; i<state.label_events.length; i++) {
        converter(state.label_events[i].artifacts);
    }
};


ClientEventController.prototype.saveClientMonitoringArtifacts = function() {
    var self = this;
    var url = 'v1/SetClientMonitoringState';

    // Convert the state to protobuf format.
    self._stateToProtobuf(self.state);

    this.grrApiService_.post(url, self.state).then(function(response) {
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
exports.ClientEventDirective = function() {
  return {
      scope: {
          "artifact": '=',
          "clientId": '=',
      },
      restrict: 'E',
      templateUrl: window.base_path+'/static/angular-components/artifact/client-event.html',
      controller: ClientEventController,
      controllerAs: 'controller',
  };
};


exports.ClientEventDirective.directive_name = 'grrClientEvents';
