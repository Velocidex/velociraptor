import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import T from '../i8n/i8n.jsx';

import api from '../core/api-service.jsx';
import Modal from 'react-bootstrap/Modal';
import StepWizard from 'react-step-wizard';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Card from 'react-bootstrap/Card';
import Table  from 'react-bootstrap/Table';
import utils from './utils.jsx';
import { HotKeys, ObserveKeys } from "react-hotkeys";
import {CancelToken} from 'axios';
import Select from 'react-select';

import NewCollectionConfigParameters from '../flows/new-collections-parameters.jsx';
import {
    NewCollectionSelectArtifacts,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
} from '../flows/new-collection.jsx';

class EventTablePaginator extends PaginationBuilder {
    PaginationSteps = [T("Configure Label Group"),
                       T("Select Artifacts"),
                       T("Configure Parameters"),
                       T("Review"),
                       T("Launch")];
}

class EventTableLabelGroup extends React.Component {
    static propTypes = {
        current_label: PropTypes.string,
        setLabel: PropTypes.func.isRequired,
        tables: PropTypes.object,
        paginator: PropTypes.object,
    }

    state = {
        // A list of all labels on the system to let the user pick
        // one.
        labels: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.loadLabels();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    loadLabels = () => {
        api.get("v1/SearchClients", {
            query: "label:*",
            limit: 1000,
            name_only: true,
        }, this.source.token).then((response) => {
            let labels = _.map(response.data.names, (x) => {
                x = x.replace(/^label:/, "");
                return x;
            });
            this.setState({labels: ["All"].concat(labels), initialized: true});
        });
    };

    // Build the label suggestion list:
    // 1. First up is the All label targetting all endpoints.
    // 2. Next at the top is the labels already targetted
    // 3. Following are all the other labels.
    createSuggestionList = () => {
        let existing_labels = [];
        let seen = {};

        let make_label = (label, table) => {
            if (_.isEmpty(table)) {
                return;
            }

            let length = table.artifacts && table.artifacts.length;
            length = length || 0;

            if (!seen[label]) {
                existing_labels.push( {
                    value: label,
                    label: label + " (" + (length || 0) + " artifacts)",
                });
                seen[label] = 1;
            }
        };

        let tables = this.props.tables;
        if(!_.isEmpty(tables)) {
            make_label("All", this.props.tables.All);
            _.each(this.props.tables, (v, k) => {
                make_label(k, v);
            });
        }

        // Now add all the other labels we know about
        _.each(this.state.labels, x=>{
            if(!seen[x]) {
                existing_labels.push({
                    value: x,
                    label: x + " (0 artifacts)",
                });
            }
        });
        return existing_labels;
    }

    render() {
        let current_label = this.props.current_label;
        let current_artifacts = this.props.tables &&
            this.props.tables[current_label] &&
            this.props.tables[current_label].artifacts;
        let existing_labels = this.createSuggestionList();
        let defaultValue = undefined;
        if (current_label) {
            defaultValue = {value: current_label};
        }
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>
                  {T("Event Monitoring: Configure Label groups")}
                </Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <h1>{T("Configuring Label")} {current_label}</h1>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Label group</Form.Label>
                    <Col sm="8">
                      <Select
                        placeholder={T("Select label to edit its event monitoring table")}
                        className="velo"
                        classNamePrefix="velo"
                        closeMenuOnSelect={true}
                        onChange={e=>{
                            this.props.setLabel(e.value);
                            return false;
                        }}
                        defaultValue={defaultValue}
                        options={existing_labels}
                      />

                    </Col>
                  </Form.Group>
                </Form>
                <Card>
                  <Card.Header>{T("Event Monitoring Label Groups")}</Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("EventMonitoringCard")}
                    </Card.Text>
                    { !_.isEmpty(current_artifacts) &&
                      <><h2>{T("Label")} { current_label}</h2>
                        <Table className="paged-table">
                          <thead><tr className="paged-table-header">
                                   <th>
                                     {T("Artifact Collected")}
                                   </th>
                                 </tr>
                          </thead>
                          <tbody>
                            {_.map(current_artifacts,
                                   (x, idx)=><tr key={idx}>
                                               <td>{x.name}</td>
                                             </tr>)}
                          </tbody>
                        </Table>
                      </>}
                  </Card.Body>
                </Card>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    step_name: "Offline Collector",
                    isFocused: _.isEmpty(this.props.current_label),
                }) }
              </Modal.Footer>
            </>
        );
    }
}

export default class NewCollectionConfigParametersWithCPU extends React.Component {
    static propTypes = {
        artifacts: PropTypes.array,
        setArtifacts: PropTypes.func,
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
        configureResourceControl: PropTypes.bool,
    };

    render() {
        return <NewCollectionConfigParameters
                 artifacts={this.props.artifacts}
                 setArtifacts={this.props.setArtifacts}
                 parameters={this.props.parameters}
                 setParameters={this.props.setParameters}
                 paginator={this.props.paginator}
                 configureResourceControl={true}
               />;
    }
}

// Used to manipulate the event tables.
export class EventTableWizard extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        onResolve: PropTypes.func.isRequired,
        onCancel: PropTypes.func,
    };

    state = {
        // Mapping between labels and their event tables.
        tables: {},

        // A selector for the current label we are working with.
        current_label: "",

        // The event table for this label. Each time the label is
        // changed we update this one.
        current_table: {artifacts:[], specs:{}},
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchEventTable();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return false;
    }

    fetchEventTable = () => {
        api.get("v1/GetClientMonitoringState", {}, this.source.token).then(resp => {
            // For historical reasons the client monitoring state
            // protobuf is more complex than it needs to be. We
            // convert it to more sensible internal representation. It
            // basically consists of one table for the "All" label
            // (i.e. all clients) and one table for each label.

            // Convert from flows_proto.ClientEventTable to an
            // internal representation (client_event_table) - see
            // utils.js for description of this internal format.
            utils.proto2tables(resp.data, table=>{
                this.setState({tables: table});
            });
        });
    }

    // Update the artifacts in the current table ... immutable gymnastics.
    setArtifacts = (artifacts) => {
        let current_table = this.state.current_table;
        let tables = Object.assign({}, this.state.tables);
        let new_table = {
            artifacts: artifacts,
            specs: current_table.specs || {},
        };
        tables[this.state.current_label] = new_table;
        this.setState({current_table: new_table, tables: tables});
    }

    setParameters = (params) => {
        let current_table = this.state.current_table;
        let tables = Object.assign({}, this.state.tables);
        let new_table = {
            artifacts: current_table.artifacts,
            specs: params,
        };
        tables[this.state.current_label] = new_table;
        this.setState({current_table: new_table, tables: tables});
    }

    // Select a table depending on the selected label. If a table does
    // not exist - create it.
    setLabel = (label) => {
        let tables = this.state.tables;
        let current_table = tables[label];

        // If no table currently exists for this label, make a new
        // one.
        if (!current_table) {
            current_table = {artifacts: [], specs: {}};
            tables[label] = current_table;
        }

        // Update the wizard parameters from the event table.
        this.setState({current_label: label,
                       current_table: current_table,
                       tables: tables});
    }

    // Let our caller know the artifact request we created.
    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }

    // Build a new event table protobuf from our internal state.
    prepareRequest = () => {
        return utils.table2protobuf(this.state.tables);
    }

    gotoStep = (step) => {
        return e=>{
            this.step.goToStep(step);
            e.preventDefault();
        };
    };

    gotoNextStep = () => {
        return e=>{
            this.step.nextStep();
            e.preventDefault();
        };
    }

    gotoPrevStep = () => {
        return e=>{
            this.step.previousStep();
            e.preventDefault();
        };
    }

    render() {
        let label = this.state.current_label;
        let keymap = {
            GOTO_LAUNCH: "ctrl+l",
            NEXT_STEP: "ctrl+right",
            PREV_STEP: "ctrl+left",
        };
        let handlers={
            GOTO_LAUNCH: this.gotoStep(5),
            NEXT_STEP: this.gotoNextStep(),
            PREV_STEP: this.gotoPrevStep(),
        };

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <HotKeys keyMap={keymap} handlers={handlers}><ObserveKeys>
              <StepWizard ref={n=>this.step=n}>
                <EventTableLabelGroup
                  current_label={this.state.current_label}
                  setLabel={this.setLabel}
                  tables={this.state.tables}
                  paginator={new EventTablePaginator(
                      "Configure Label Group",
                      "Event Monitoring: Configure client labels")}
                />

                <NewCollectionSelectArtifacts
                  paginator={new EventTablePaginator(
                      "Select Artifacts",
                      T("Event Monitoring: Select artifacts to collect from label group ") + label,
                      ()=>false)}
                  artifacts={this.state.current_table.artifacts}
                  artifactType="CLIENT_EVENT"
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  parameters={this.state.current_table.specs}
                  setParameters={this.setParameters}
                  artifacts={this.state.current_table.artifacts}
                  setArtifacts={this.setArtifacts}
                  configureResourceControl={true}
                  paginator={new EventTablePaginator(
                      "Configure Parameters",
                      T("Event Monitoring: Configure artifact parameters for label group ") + label)}
                />

                <NewCollectionRequest
                  paginator={new EventTablePaginator(
                      "Review",
                      T("Event Monitoring: Review new event tables"))}
                  request={this.prepareRequest()} />

                <NewCollectionLaunch
                  artifacts={this.state.current_table.artifacts}
                  paginator={new PaginationBuilder(
                      "Launch",
                      T("New Collection: Launch collection"))}
                  launch={this.launch} />

              </StepWizard>
              </ObserveKeys></HotKeys>
            </Modal>
        );
    }
};


class ServerEventTablePaginator extends PaginationBuilder {
    PaginationSteps = ["Select Artifacts", "Configure Parameters",
                       "Review", "Launch"];
}

// Used to manipulate the event tables for the server. The server does
// not use labels so has just a single default event table.
export class ServerEventTableWizard extends React.Component {
    static propTypes = {
        onResolve: PropTypes.func.isRequired,
        onCancel: PropTypes.func,
    };

    state = {
        // Mapping between labels and their event tables.
        tables: {},

        // A selector for the current label we are working with.
        current_label: "",

        // The event table for this label. Each time the label is
        // changed we update this one.
        current_table: {artifacts:[], specs:{}},
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchEventTable();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return false;
    }

    fetchEventTable = () => {
        api.get("v1/GetServerMonitoringState", {}, this.source.token).then(resp => {
            let empty_table = {All: {artifacts: [], specs: {}}};
            this.setState({tables: empty_table, current_table: empty_table.All});

            // Pretend this is the same as the client event
            // table. Since the server events do not do labels, just
            // make them all go under the "All" label.
            utils.proto2tables({artifacts: resp.data}, table=>{
                this.setState({tables: table, current_table: table.All});
            });
        });
    }

    // Update the artifacts in the current table.
    setArtifacts = (artifacts) => {
        let current_table = this.state.current_table;
        current_table.artifacts = artifacts;
        this.setState({current_table: current_table});
    }

    setParameters = (params) => {
        let current_table = this.state.current_table;
        current_table.specs = params;
        this.setState({current_table: current_table});
    }

    // Let our caller know the artifact request we created.
    launch = () => {
        let request = this.prepareRequest();
        this.props.onResolve(request.artifacts);
    }

    // Build a new event table protobuf from our internal state.
    prepareRequest = () => {
        return utils.table2protobuf(this.state.tables);
    }

    gotoStep = (step) => {
        return e=>{
            this.step.goToStep(step);
            e.preventDefault();
        };
    };

    gotoNextStep = () => {
        return e=>{
            this.step.nextStep();
            e.preventDefault();
        };
    }

    gotoPrevStep = () => {
        return e=>{
            this.step.previousStep();
            e.preventDefault();
        };
    }

    render() {
        let keymap = {
            GOTO_LAUNCH: "ctrl+l",
            NEXT_STEP: "ctrl+right",
            PREV_STEP: "ctrl+left",
        };
        let handlers={
            GOTO_LAUNCH: this.gotoStep(4),
            NEXT_STEP: this.gotoNextStep(),
            PREV_STEP: this.gotoPrevStep(),
        };

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <HotKeys keyMap={keymap} handlers={handlers}><ObserveKeys>
              <StepWizard ref={n=>this.step=n}>
                <NewCollectionSelectArtifacts
                  paginator={new ServerEventTablePaginator(
                      "Select Artifacts",
                      T("Server Event Monitoring: Select artifacts to collect on the server"),
                      ()=>false)}
                  artifacts={this.state.current_table.artifacts}
                  artifactType="SERVER_EVENT"
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  parameters={this.state.current_table.specs}
                  setParameters={this.setParameters}
                  artifacts={this.state.current_table.artifacts}
                  setArtifacts={this.setArtifacts}
                  paginator={new ServerEventTablePaginator(
                      "Configure Parameters",
                      T("Server Event Monitoring: Configure artifact parameters for server"))}
                />

                <NewCollectionRequest
                  paginator={new ServerEventTablePaginator(
                      "Review",
                      T("Server Event Monitoring: Review new event tables"))}
                  request={this.prepareRequest().artifacts} />

                <NewCollectionLaunch
                  artifacts={this.state.current_table.artifacts}
                  paginator={new PaginationBuilder(
                      "Launch",
                      T("New Collection: Launch collection"))}
                  launch={this.launch} />

              </StepWizard>
              </ObserveKeys></HotKeys>
            </Modal>
        );
    }
};
