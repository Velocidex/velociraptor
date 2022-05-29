import './events.css';

import React from 'react';
import PropTypes from 'prop-types';
import axios from 'axios';
import _ from 'lodash';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Dropdown from 'react-bootstrap/Dropdown';
import "react-datepicker/dist/react-datepicker.css";

import { EventTableWizard, ServerEventTableWizard } from './event-table.js';
import Container from  'react-bootstrap/Container';
import VeloReportViewer from "../artifacts/reporting.js";
import Modal from 'react-bootstrap/Modal';
import VeloAce from '../core/ace.js';
import { SettingsButton } from '../core/ace.js';
import VeloTimestamp from "../utils/time.js";
import EventTimelineViewer from "./timeline-viewer.js";
import EventNotebook from "./event-notebook.js";
import { get_notebook_id} from "./event-notebook.js";
import DeleteNotebookDialog from '../notebooks/notebook-delete.js';
import T from '../i8n/i8n.js';

import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';


const mode_raw_data = "Raw Data";
const mode_logs = "Logs";
const mode_report = "Report";
const mode_notebook = "Notebook";


class InspectRawJson extends React.PureComponent {
    static propTypes = {
        client: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchEventTable();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return false;
    }

    state = {
        raw_json: "",
    }

    fetchEventTable = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();

        // The same file is used by both server and client event
        // tables - only difference is that the server's client id is
        // "server".
        let client_id = (this.props.client &&
                         this.props.client.client_id) || "server";
        if (client_id === "server") {
            api.get("v1/GetServerMonitoringState", {},
                    this.source.token).then(resp => {
                        if (resp.cancel) return;

                        let table = resp.data;
                        delete table["compiled_collector_args"];
                        this.setState({
                            raw_json: JSON.stringify(table, null, 2),
                        });
                    });
            return;
        }
        api.get("v1/GetClientMonitoringState", {
            client_id: client_id,
        }, this.source.token).then(resp => {
            if (resp.cancel) return;

            let table = resp.data;
            delete table.artifacts["compiled_collector_args"];
            _.each(table.label_events, x=> {
                delete x.artifacts["compiled_collector_args"];
            });

            this.setState({raw_json: JSON.stringify(table, null, 2)});
        });
    }

    aceConfig = (ace) => {
        // Hold a reference to the ace editor.
        this.setState({ace: ace});
    };

    render() {
        let client_id = (this.props.client &&
                         this.props.client.client_id) || "server";
        return (
            <>
              <Modal show={true}
                     className="full-height"
                     enforceFocus={false}
                     scrollable={true}
                     dialogClassName="modal-90w"
                     onHide={this.props.onClose}>
                <Modal.Header closeButton>
                  <Modal.Title>
                    Raw { client_id !== "server" ? "Client " : "Server "}
                    Monitoring Table JSON
                  </Modal.Title>
                </Modal.Header>
                <Modal.Body>
                  <VeloAce text={this.state.raw_json}
                           mode="json"
                           aceConfig={this.aceConfig}
                           options={{
                               wrap: true,
                               autoScrollEditorIntoView: true,
                               minLines: 10,
                               maxLines: 1000,
                           }}
                  />
                </Modal.Body>

                <Modal.Footer>
                  <Navbar className="w-100 justify-content-between">
                    <ButtonGroup className="float-left">
                      <SettingsButton ace={this.state.ace}/>
                    </ButtonGroup>

                    <ButtonGroup className="float-right">
                      <Button variant="secondary"
                              onClick={this.props.onClose} >
                        Close
                      </Button>
                    </ButtonGroup>
                  </Navbar>
                </Modal.Footer>
              </Modal>
            </>
        );
    };
}

class EventMonitoring extends React.Component {
    static propTypes = {
        client: PropTypes.object,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchEventResults();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let client_id = (this.props.client &&
                         this.props.client.client_id) || "server";
        let prev_client_id = (prevProps.client &&
                              prevProps.client.client_id) || "server";
        if (client_id !== prev_client_id) {
            this.fetchEventResults();
        }
    }

    state = {
        // Selected artifact we are currently viewing.
        artifact: {},

        // All available artifacts
        available_artifacts: [],

        showDateSelector: false,
        showEventTableWizard: false,
        showExportNotebook: false,
        showDeleteNotebook: false,

        // Refreshed from the server
        event_monitoring_table: {},
        showEventMonitoringPopup: false,

        // Are we viewing the report or the raw data?
        mode: mode_raw_data,

        // A callback for child components to add toolbar buttons in
        // this component.
        buttonsRenderer: ()=>{},


        start_time: 0,
        end_time: 0,
    }

    setTimeRange = (start_time, end_time) => {
        this.setState({
            start_time: start_time, end_time: end_time
        });
    }

    fetchEventResults = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();
        let client_id = this.props.client.client_id || "server";

        api.post("v1/ListAvailableEventResults", {
            client_id: client_id,
        }, this.source.token).then(resp => {
            if (resp.cancel) return;

            let router_artifact = this.props.match && this.props.match.params &&
                this.props.match.params.artifact;
            if (router_artifact) {
                let logs = resp.data.logs || [];
                for(let i=0; i<logs.length;i++) {
                    let log=logs[i];
                    if (log.artifact === router_artifact) {
                        this.setState({artifact: log});
                    }
                }
            }

            this.setState({available_artifacts: resp.data.logs});
        });
    }

    setArtifact = (artifact) => {
        this.setState({artifact: artifact});
        let client_id = (this.props.client &&
                         this.props.client.client_id) || "server";
        this.props.history.push('/events/' + client_id + '/' + artifact.artifact);
    }

    setEventTable = (request) => {
        api.post("v1/SetClientMonitoringState", request,
                 this.source.token).then(resp=>{
            this.setState({showEventTableWizard: false});
        });
    }

    setServerEventTable = (request) => {
        api.post("v1/SetServerMonitoringState", request,
                 this.source.token).then(resp=>{
            this.setState({showServerEventTableWizard: false});
        });
    }

    render() {
        let timestamp_renderer = (cell, row, rowIndex) => {
            return (
                <VeloTimestamp usec={cell}/>
            );
        };

        let renderers = {
            "_ts": timestamp_renderer,
            "Timestamp": timestamp_renderer,
            "client_time": timestamp_renderer,
        };

        let column_types = this.state.artifact && this.state.artifact.definition &&
            this.state.artifact.definition.column_types;

        let client_id = (this.props.client &&
                         this.props.client.client_id) || "server";
        return (
            <>
              {this.state.showEventMonitoringPopup &&
               <InspectRawJson client={this.props.client}
                               onClose={()=>this.setState({showEventMonitoringPopup: false})}
               /> }
              {this.state.showEventTableWizard &&
               <EventTableWizard client={this.state.client}
                                 onCancel={()=>this.setState({showEventTableWizard: false})}
                                 onResolve={this.setEventTable}
               />}
              {this.state.showServerEventTableWizard &&
               <ServerEventTableWizard
                 onCancel={()=>this.setState({showServerEventTableWizard: false})}
                 onResolve={this.setServerEventTable}
               />}

              <Navbar className="artifact-toolbar justify-content-between">
                <ButtonGroup>
                  { client_id === "server" || client_id === "" ?
                    <>
                      <Button title={T("Update server monitoring table")}
                              onClick={() => this.setState({showServerEventTableWizard: true})}
                              variant="default">
                        <FontAwesomeIcon icon="edit"/>
                      </Button>
                      <Button title={T("Show server monitoring tables")}
                              onClick={() => this.setState({showEventMonitoringPopup: true})}
                              variant="default">
                        <FontAwesomeIcon icon="binoculars"/>
                      </Button>

                    </>:
                    <>
                      <Button title={T("Update client monitoring table")}
                              onClick={() => this.setState({showEventTableWizard: true})}
                              variant="default">
                        <FontAwesomeIcon icon="edit"/>
                      </Button>
                      <Button title={T("Show client monitoring tables")}
                              onClick={() => this.setState({showEventMonitoringPopup: true})}
                              variant="default">
                        <FontAwesomeIcon icon="binoculars"/>
                      </Button>
                    </>
                  }
                  { this.state.buttonsRenderer() }
                  <Dropdown title={this.state.artifact.artifact ||
                                   T("Select artifact")} variant="default">
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="book"/>
                      <span className="button-label">
                        {this.state.artifact.artifact || T("Select artifact")}
                      </span>
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      { _.map(this.state.available_artifacts, (x, idx) => {
                          let active_artifact = this.state.artifact &&
                              this.state.artifact.artifact;
                          return <Dropdown.Item
                                   key={idx}
                                   title={x.artifact}
                                   active={x.artifact === active_artifact}
                                   onClick={() => {
                                       this.setArtifact(x);
                                   }}>
                                   {x.artifact}
                                 </Dropdown.Item>;
                      })}
                    </Dropdown.Menu>
                  </Dropdown>
                </ButtonGroup>

                <ButtonGroup className="float-right">
                  { this.state.mode === mode_notebook &&
                    <Button title={T("Delete Notebook")}
                            onClick={() => this.setState({showDeleteNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="trash"/>
                    </Button>
                  }
                  <Dropdown title="mode" variant="default">
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="book"/>
                      <span className="button-label">{T(this.state.mode)}</span>
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      <Dropdown.Item
                        title={T(mode_raw_data)}
                        active={this.state.mode === mode_raw_data}
                        onClick={() => this.setState({mode: mode_raw_data})}>
                        {T(mode_raw_data)}
                      </Dropdown.Item>
                      <Dropdown.Item
                        title={T(mode_logs)}
                        active={this.state.mode === mode_logs}
                        onClick={() => this.setState({mode: mode_logs})}>
                        {T(mode_logs)}
                      </Dropdown.Item>
                      <Dropdown.Item
                        title={T(mode_report)}
                        active={this.state.mode === mode_report}
                        onClick={() => this.setState({mode: mode_report})}>
                        {T(mode_report)}
                      </Dropdown.Item>
                      <Dropdown.Item
                        title={T(mode_notebook)}
                        active={this.state.mode === mode_notebook}
                        onClick={() => this.setState({mode: mode_notebook})}>
                        {T(mode_notebook)}
                      </Dropdown.Item>
                    </Dropdown.Menu>
                  </Dropdown>
                </ButtonGroup>
              </Navbar>
              { (this.state.mode === mode_raw_data ||
                 this.state.mode === mode_logs) && this.state.artifact.artifact &&
                <Container className="event-report-viewer">
                <EventTimelineViewer
                  toolbar={x=>this.setState({buttonsRenderer: x})}
                  client_id={client_id}
                  artifact={this.state.artifact.artifact}
                  mode={this.state.mode}
                  renderers={renderers}
                  column_types={column_types}
                  time_range_setter={this.setTimeRange}
                 />
                </Container> }

            { this.state.mode === mode_report &&
              <Container className="event-report-viewer">
                { this.state.artifact.artifact ?
                  <VeloReportViewer
                    artifact={this.state.artifact.artifact}
                    type="CLIENT_EVENT"
                    client={this.props.client}
                  /> :
                  <div className="no-content">Please select an artifact to view above.</div>
                }
              </Container>
            }

            { this.state.mode === mode_notebook &&
              <Container className="event-report-viewer">
                { this.state.artifact.artifact ?
                  <EventNotebook
                    artifact={this.state.artifact.artifact}
                    client_id={client_id}
                    start_time={this.state.start_time}
                  /> :
                  <div className="no-content">Please select an artifact to view above.</div>
                }
              </Container>
            }
              { this.state.showDeleteNotebook &&
                <DeleteNotebookDialog
                  notebook_id={get_notebook_id(
                      this.state.artifact.artifact, client_id)}
                  onClose={e=>{
                      this.setState({showDeleteNotebook: false});
                  }}/>
              }
            </>
        );
    }
};

export default withRouter(EventMonitoring);
