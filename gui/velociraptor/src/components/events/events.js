import './events.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Dropdown from 'react-bootstrap/Dropdown';
import DatePicker  from "react-datepicker";
import "react-datepicker/dist/react-datepicker.css";

import Form from 'react-bootstrap/Form';
import { EventTableWizard, ServerEventTableWizard } from './event-table.js';
import Container from  'react-bootstrap/Container';
import VeloReportViewer from "../artifacts/reporting.js";
import Modal from 'react-bootstrap/Modal';
import VeloAce from '../core/ace.js';
import { SettingsButton } from '../core/ace.js';

import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';


class InspectRawJson extends React.PureComponent {
    static propTypes = {
        client: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.fetchEventTable();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return false;
    }

    state = {
        raw_json: "",
    }

    fetchEventTable = () => {
        api.get("v1/GetClientMonitoringState").then(resp => {
            let table = resp.data;
            delete table.artifacts["compiled_collector_args"];
            _.each(table.label_events, x=> {
                delete x.artifacts["compiled_collector_args"];
            });

            this.setState({raw_json: JSON.stringify(resp.data, null, 2)});
        });
    }

    aceConfig = (ace) => {
        // Hold a reference to the ace editor.
        this.setState({ace: ace});
    };

    render() {
        return (
            <>
              <Modal show={true}
                     className="full-height"
                     enforceFocus={false}
                     scrollable={true}
                     dialogClassName="modal-90w"
                     onHide={this.props.onClose}>
                <Modal.Header closeButton>
                  <Modal.Title>Raw Monitoring Table JSON</Modal.Title>
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
                              onClick={() => this.setState({show: false})} >
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


function format_day(date) {
    return date.getFullYear().toString().padStart(4, "0") + "-" +
        (date.getMonth()+1).toString().padStart(2, "0") + "-" +
        date.getDate().toString().padStart(2, "0");
}

// Convert from date in local time to a utc range covering that day.
function get_day_range(date) {
    let utc = format_day(date);

    return {
        start_ts: (new Date(utc + "T00:00:00Z")).getTime()/1000,
        end_ts: (new Date(utc + "T23:59:59Z")).getTime()/1000
    };
};

class EventMonitoring extends React.Component {
    static propTypes = {
        client: PropTypes.object,
    };

    componentDidMount = () => {
        this.fetchEventResults();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let client_id = this.props.client && this.props.client.client_id;
        let prev_client_id = prevProps.client && prevProps.client.client_id;
        if (client_id !== prev_client_id) {
            this.fetchEventResults();
        }
    }

    state = {
        // Selected artifact we are currently viewing.
        artifact: {},

        // All available artifacts
        available_artifacts: [],

        current_time: new Date(),
        activeStartDate: new Date(),

        showDateSelector: false,
        showEventTableWizard: false,


        // Refreshed from the server
        event_monitoring_table: {},
        showEventMonitoringPopup: false,
    }

    fetchEventResults = () => {
        api.post("v1/ListAvailableEventResults", {
            client_id: this.props.client.client_id,
        }).then(resp => {
            let router_artifact = this.props.match && this.props.match.params &&
                this.props.match.params.artifact;
            if (router_artifact) {
                let logs = resp.data.logs || [];
                for(let i=0; i<logs.length;i++) {
                    let log=logs[i];
                    if (log.artifact === router_artifact &&
                        log.timestamps.length > 0) {
                        let last_time = new Date(
                            log.timestamps[log.timestamps.length-1]*1000);

                        this.setState({artifact: log,
                                       activeStartDate: last_time,
                                       current_time: last_time});
                    }
                }
            }

            this.setState({available_artifacts: resp.data.logs});
        });
    }

    dateDisabled = ({activeStartDate, date, view }) => !this.isDateCovered(date);

    setDay = (value) => {
        this.setState({current_time: value});
    }

    isDateCovered = (date) => {
        if (!this.state.artifact || !this.state.artifact.timestamps) {
            return false;
        }

        const {start_ts, end_ts} = get_day_range(date);

        // Tile is enabled if there is a timestamp in the range.
        for (let i=0; i<=this.state.artifact.timestamps.length; i++) {
            let ts=this.state.artifact.timestamps[i];
            if ((start_ts <= ts) && (ts < end_ts)) {
                return true;
            }
        };

        return false;
    }

    setArtifact = (artifact) => {
        let current_time = new Date();

        // Set the timestamp to the latest available time.
        if (artifact.timestamps.length > 0) {
            current_time = new Date(artifact.timestamps[artifact.timestamps.length-1] * 1000);
        }

        this.setState({artifact: artifact, current_time: current_time});
        let client_id = this.props.client && this.props.client.client_id;
        client_id = client_id || "server";
        this.props.history.push('/events/' + client_id + '/' + artifact.artifact);
    }

    setEventTable = (request) => {
        api.post("v1/SetClientMonitoringState", request).then(resp=>{
            this.setState({showEventTableWizard: false});
        });
    }

    setServerEventTable = (request) => {
        api.post("v1/SetServerMonitoringState", request).then(resp=>{
            this.setState({showServerEventTableWizard: false});
        });
    }

    render() {
        const {start_ts, end_ts} = get_day_range(this.state.current_time);

        // Check if there are any results in the current_time range
        let is_date_covered = this.isDateCovered(this.state.current_time);

        let client_id = this.props.client && this.props.client.client_id;
        return (
            <>
              {this.state.showEventMonitoringPopup &&
               <InspectRawJson client={this.state.client}
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
                    <Button title="Update server monitoring table"
                            onClick={() => this.setState({showServerEventTableWizard: true})}
                            variant="default">
                      <FontAwesomeIcon icon="edit"/>
                    </Button>   :

                    <Button title="Update client monitoring table"
                            onClick={() => this.setState({showEventTableWizard: true})}
                            variant="default">
                      <FontAwesomeIcon icon="edit"/>
                    </Button> }
                  <Button title="Show client monitoring tables"
                          onClick={() => this.setState({showEventMonitoringPopup: true})}
                          variant="default">
                    <FontAwesomeIcon icon="binoculars"/>
                  </Button>

                  <Dropdown title={this.state.artifact.artifact ||
                                   "Select artifact"} variant="default">
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="book"/>
                      <span className="button-label">
                        {this.state.artifact.artifact || "Select artifact"}
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
                  <DatePicker
                    filterDate={x=>this.isDateCovered(x)}
                    selected={this.state.current_time}
                    onSelect={this.setDay}
                    disabled={_.isEmpty(this.state.artifact) ||
                              _.isEmpty(this.state.artifact.timestamps)}
                    disabledKeyboardNavigation
                    customInput={
                        <Form.Control placeholder="Select artifact to view"
                          className="time-selector" size="sm">
                        </Form.Control>
                    }
                  />
                </ButtonGroup>
              </Navbar>
              <Container className="event-report-viewer">
                { (this.state.artifact.artifact && is_date_covered) ?
                  <VeloReportViewer
                    artifact={this.state.artifact.artifact}
                    type="CLIENT_EVENT"
                    params={{
                        start_time: start_ts,
                        end_time: end_ts,
                    }}
                    client={this.props.client}
                  /> :
                  <div className="no-content">Please select an artifact to view above.</div>
                }
              </Container>
            </>
        );
    }
};

export default withRouter(EventMonitoring);
