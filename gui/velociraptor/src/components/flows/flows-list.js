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

import { NavLink } from "react-router-dom";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HotKeys } from "react-hotkeys";
import { withRouter } from "react-router-dom";

import Modal from 'react-bootstrap/Modal';
import axios from 'axios';


class DeleteFlowDialog extends React.PureComponent {
    static propTypes = {
        client: PropTypes.object,
        flow: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    startDeleteFlow = () => {
        let client_id = this.props.client && this.props.client.client_id;
        let flow_id = this.props.flow && this.props.flow.session_id;

        if (flow_id && client_id) {
            api.post("v1/CollectArtifact", {
                client_id: "server",   // This collection happens on the server.
                artifacts: ["Server.Utils.DeleteFlow"],
                parameters: {"env": [
                    { "key": "FlowId", "value": flow_id},
                    { "key": "ClientId", "value": client_id},
                    { "key": "ReallyDoIt", "value": "Y"}]},
            }).then((response) => {
                this.props.onClose();
            });
        }
    }

    render() {
        let collected_artifacts = this.props.flow.artifacts_with_results || [];
        let artifacts = collected_artifacts.join(",");
        let total_bytes = this.props.flow.total_uploaded_bytes/1024/1024 || 0;
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Permanently delete collection</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                You are about to permanently delete the artifact collection
                <b>{this.props.flow.session_id}</b>.
                <br/>
                This collection was the artifacts <b>{artifacts}</b>
                <br/><br/>

                We expect to free up { total_bytes.toFixed(0) } Mb.
                </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.startDeleteFlow}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

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
        showDeleteWizard: false,
        initialized_from_parent: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        if (this.interval) {
            clearInterval(this.interval);
        }
        if (this.recrusive_interval) {
            clearInterval(this.recrusive_interval);
        }
    }

    // Set the table in focus when the component mounts for the first time.
    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        if (!this.state.initialized_from_parent && selected_flow) {
            const el = document.getElementById(selected_flow);
            if (el) {
                this.setState({initialized_from_parent: true});
                el.focus();
            }
        }
    }

    setCollectionRequest = (request) => {
        // Make a request to the start the flow on this client.
        request.client_id = this.props.client.client_id;
        api.post("v1/CollectArtifact", request).then((response) => {
            // When the request is done force our parent to refresh.
            this.props.fetchFlows();

            // Only disable wizards if the request was successful.
            this.setState({showWizard: false,
                           showOfflineWizard: false,
                           showCopyWizard: false});
        });
    }

    archiveButtonClicked = () => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let flow_id = this.props.selected_flow && this.props.selected_flow.session_id;

        if (client_id && flow_id) {
            api.post("v1/ArchiveFlow", {
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
            api.post("v1/CancelFlow", {
                client_id: client_id, flow_id: flow_id
            }).then((response) => {
                this.props.fetchFlows();
            });
        }
    }

    // Navigates to the next row to the one that is highlighted
    gotoNextRow = () => {
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        for(let i=0; i<this.node.table.props.data.length; i++) {
            let row = this.node.table.props.data[i];
            if (row.session_id === selected_flow) {
                // Last row
                if (i+1 >= this.node.table.props.data.length) {
                    return;
                }
                let next_row = this.node.table.props.data[i+1];
                this.props.setSelectedFlow(next_row);

                // Scroll the new row into view.
                const el = document.getElementById(next_row.session_id);
                if (el) {
                    el.scrollIntoView();
                    el.focus();
                }
            }
        };
    }
    gotoPrevRow = () => {
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        for(let i=0; i<this.node.table.props.data.length; i++) {
            let row = this.node.table.props.data[i];
            if (row.session_id === selected_flow) {
                // First row
                if(i===0){
                    return;
                }
                let prev_row = this.node.table.props.data[i-1];
                this.props.setSelectedFlow(prev_row);

                // Scroll the new row into view.
                const el = document.getElementById(prev_row.session_id);
                if (el) {
                    el.scrollIntoView();
                    el.focus();
                }
            }
        };
    }

    gotoTab = (tab) => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        this.props.history.push(
            "/collected/" + client_id + "/" + selected_flow + "/" + tab);
    }

    render() {
        let client_id = this.props.client && this.props.client.client_id;

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
            {dataField: "session_id", text: "FlowId",
             formatter: (cell, row) => {
                 return <NavLink
                          tabIndex="0"
                          id={cell}
                          to={"/collected/" + client_id + "/" + cell}>{cell}
                        </NavLink>;
             }},
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
             sort: true, filtered: true},
            {dataField: "total_uploaded_bytes", text: "Uploaded Mb",
             align: (column, colIndex) => 'right',
             sort: true, sortNumeric: true,
             formatter: (cell, row) => {
                 if (cell) {
                     return (cell /1024/1024).toFixed(0);
                 }
                 return <></>;
             }},
            {dataField: "total_collected_rows", text: "Rows",
             sort: true, sortNumeric: true,
             align: (column, colIndex) => 'right'}
        ]);

        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: row=>this.props.setSelectedFlow(row),
            selected: [selected_flow],
        };

        // When running on the server we have some special GUI.
        let isServer = client_id === "server";
        let KeyMap = {
            GOTO_RESULTS: {
                name: "Display server dashboard",
                sequence: "r",
            },
            GOTO_LOGS: "l",
            GOTO_OVERVIEW: "o",
            GOTO_UPLOADS: "u",
            NEXT: "n",
            PREVIOUS: "p",
            COLLECT: "c",
        };

        let keyHandlers={
            GOTO_RESULTS: (e)=>this.gotoTab("results"),
            GOTO_LOGS: (e)=>this.gotoTab("logs"),
            GOTO_UPLOADS: (e)=>this.gotoTab("uploads"),
            GOTO_OVERVIEW: (e)=>this.gotoTab("overview"),
            NEXT: this.gotoNextRow,
            PREVIOUS: this.gotoPrevRow,
            COLLECT: ()=>this.setState({showWizard: true}),
        };

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={this.props.client}
                  flow={this.props.selected_flow}
                  onClose={(e) => this.setState({showDeleteWizard: false})}/>
              }
              { this.state.showWizard &&
                <NewCollectionWizard
                  client={this.props.client}
                  onCancel={(e) => this.setState({showWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showCopyWizard &&
                <NewCollectionWizard
                  client={this.props.client}
                  baseFlow={this.props.selected_flow}
                  onCancel={(e) => this.setState({showCopyWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showOfflineWizard &&
                <OfflineCollectorWizard
                  onCancel={(e) => this.setState({showOfflineWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="New Collection"
                          onClick={() => this.setState({showWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Archive Artifact Collection"
                          onClick={this.archiveButtonClicked}
                          variant="default">
                    <FontAwesomeIcon icon="archive"/>
                  </Button>

                  <Button title="Delete Artifact Collection"
                          onClick={()=>this.setState({showDeleteWizard: true}) }
                          variant="default">
                    <FontAwesomeIcon icon="eraser"/>
                  </Button>

                  { this.props.selected_flow.state === "RUNNING" &&
                    <Button title="Cancel Artifact Collection"
                            onClick={this.cancelButtonClicked}
                            variant="default">
                    <FontAwesomeIcon icon="stop"/>
                    </Button>
                  }
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
                <HotKeys keyMap={KeyMap} handlers={keyHandlers}>
                  <BootstrapTable
                    hover
                    condensed
                    ref={ n => this.node = n }
                    keyField="session_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={this.props.flows}
                    columns={columns}
                    selectRow={ selectRow }
                    filter={ filterFactory() }
                  />
                </HotKeys>
              </div>
            </>
        );
    }
};

export default withRouter(FlowsList);
