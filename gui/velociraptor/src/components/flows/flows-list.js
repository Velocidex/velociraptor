import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import BootstrapTable from 'react-bootstrap-table-next';
import VeloTimestamp from "../utils/time.js";
import filterFactory from 'react-bootstrap-table2-filter';

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';

import api from '../core/api-service.js';
import { formatColumns } from "../core/table.js";

import NewCollectionWizard from './new-collection.js';
import OfflineCollectorWizard from './offline-collector.js';

import StepWizard from 'react-step-wizard';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

class FlowsList extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        flows: PropTypes.array,
        setSelectedFlow: PropTypes.func,
        selected_flow: PropTypes.object,
        fetchFlows: PropTypes.func,
    };

    state = {
        showWizard: false,
        showCopyWizard: false,
        showOfflineWizard: false,
    }

    setCollectionRequest = (request) => {
        // Make a request to the start the flow on this client.
        request.client_id = this.props.client.client_id;
        api.post("api/v1/CollectArtifact", request).then((response) => {
            // When the request is done force our parent to refresh.
            this.props.fetchFlows();
        });

        this.setState({showWizard: false, showCopyWizard: false});
    }

    deleteButtonClicked = () => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let flow_id = this.props.selected_flow && this.props.selected_flow.session_id;

        if (client_id && flow_id) {
            api.post("api//v1/ArchiveFlow", {
                client_id: client_id, flow_id: flow_id
            }).then((response) => {
                this.props.fetchFlows();
            });
        }
    }

    cancelButtonClicked = () => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let flow_id = this.props.selected_flow && this.props.selected_flow.session_id;

        if (client_id && flow_id) {
            api.post("api//v1/CancelFlow", {
                client_id: client_id, flow_id: flow_id
            }).then((response) => {
                this.props.fetchFlows();
            });
        }
    }

    launchOfflineCollector = () => {
        this.setState({showOfflineWizard: false});
    }

    render() {
        let columns = formatColumns([
            {dataField: "state", text: "State", sort: true,
             formatter: (cell, row) => {
                 if (cell === "FINISHED") {
                     return <FontAwesomeIcon icon="check"/>;
                 }
                 if (cell === "RUNNING") {
                     return <FontAwesomeIcon icon="hourglass"/>;
                 }
                 return <FontAwesomeIcon icon="exclamation"/>;
             }
            },
            {dataField: "session_id", text: "FlowId"},
            {dataField: "request.artifacts", text: "Artifacts",
             sort: true, filtered: true,
             formatter: (cell, row) => {
                return _.map(cell, function(item, idx) {
                    return <div key={idx}>{item}</div>;
                });
            }},
            {dataField: "create_time", text: "Created", sort: true,
             formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell / 1000}/>;
             }
            },
            {dataField: "active_time", text: "Last Active", sort: true,
             formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell / 1000}/>;
             }
            },
            {dataField: "request.creator", text: "Creator",
             sort: true, filtered: true}
        ]);

        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: function(row) {
                this.props.setSelectedFlow(row);
            }.bind(this),
            selected: [selected_flow],
        };

        // When running on the server we have some special GUI.
        let client_id = this.props.client && this.props.client.client_id;
        let isServer = client_id == "server";


        return (
            <>
              { this.state.showWizard &&
                <NewCollectionWizard
                  onCancel={(e) => this.setState({showWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showCopyWizard &&
                <NewCollectionWizard
                  baseFlow={this.props.selected_flow}
                  onCancel={(e) => this.setState({showCopyWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showOfflineWizard &&
                <OfflineCollectorWizard
                  onCancel={(e) => this.setState({showOfflineWizard: false})}
                  onResolve={this.launchOfflineCollector} />
              }

              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="New Collection"
                          onClick={() => this.setState({showWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Delete Artifact Collection"
                          onClick={this.deleteButtonClicked}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>
                  <Button title="Cancel Artifact Collection"
                          onClick={this.cancelButtonClicked}
                          variant="default">
                    <FontAwesomeIcon icon="stop"/>
                  </Button>
                  <Button title="Copy Collection"
                          onClick={() => this.setState({showCopyWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="copy"/>
                  </Button>

                  { isServer &&
                    <Button title="Build offline collector"
                            onClick={() => this.setState({showOfflineWizard: true})}
                            variant="default">
                      <FontAwesomeIcon icon="paper-plane"/>
                    </Button>
                  }

                </ButtonGroup>
              </Navbar>

              <div className="fill-parent no-margins toolbar-margin selectable">
                <BootstrapTable
                  hover
                  condensed
                  keyField="session_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.props.flows}
                  columns={columns}
                  selectRow={ selectRow }
                  filter={ filterFactory() }
                />
              </div>
            </>
        );
    }
};

export default FlowsList;
