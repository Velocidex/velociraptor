import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import api from '../core/api-service.js';
import Modal from 'react-bootstrap/Modal';
import StepWizard from 'react-step-wizard';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Card from 'react-bootstrap/Card';
import Table  from 'react-bootstrap/Table';
import utils from './utils.js';

import { Typeahead, Token } from 'react-bootstrap-typeahead';
import "react-bootstrap-typeahead/css/Typeahead.css";

import {
    NewCollectionSelectArtifacts,
    NewCollectionConfigParameters,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
} from '../flows/new-collection.js';

class EventTablePaginator extends PaginationBuilder {
    PaginationSteps = ["Configure Label Group",
                       "Select Artifacts", "Configure Parameters",
                       "Review", "Launch"];
}

class EventTableLabelGroup extends React.Component {
    static propTypes = {
        current_label: PropTypes.object,
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
        this.loadLabels();
    }

    loadLabels = () => {
        api.get("v1/SearchClients", {
            query: "label:*",
            limit: 100,
            type: 1,
        }).then((response) => {
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
        let idx = 0;

        let make_label = (label, table) => {
            let length = table.artifacts && table.artifacts.length;
            length = length || 0;

            if (!seen[label]) {
                existing_labels.push( {
                    label: label,
                    name: label + " (" + (length || 0) + " artifacts)",
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
                    label: x,
                    name: x + " (0 artifacts)",
                    id: idx,
                });
            }
        });
        return existing_labels;
    }

    render() {
        let current_label = this.props.label;
        let current_artifacts = this.props.tables &&
            this.props.tables[current_label.label] &&
            this.props.tables[current_label.label].artifacts;
        let existing_labels = this.createSuggestionList();
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>Event Monitoring: Configure Label groups</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <h1>Configuring Label {current_label.label}</h1>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Label group</Form.Label>
                    <Col sm="8">
                      <Typeahead
                        id="label-selector"
                        autoFocus={true}
                        clearButton={true}
                        placeholder="Select label to edit its event monitoring table"
                        renderToken={(option, { onRemove }, index) => (
                                <Token
                                  key={index}
                                  onRemove={onRemove}
                                  option={option}>
                                  {`${option.name}`}
                                </Token>)}
                        labelKey="name"
                        onChange={x=>{
                            if (!_.isEmpty(x)) {
                                this.props.setLabel(x[0]);
                            }
                        }}
                        options={existing_labels}
                      >
                      </Typeahead>
                    </Col>
                  </Form.Group>
                </Form>
                <Card>
                  <Card.Header>Event Monitoring Label Groups</Card.Header>
                  <Card.Body>
                    <Card.Text>
                      Event monitoring targets specific label groups.
                      Select a label group above to configure specific
                      event artifacts targetting that group.
                    </Card.Text>
                    { !_.isEmpty(current_artifacts) &&
                      <><h2>Label { current_label.label}</h2>
                        <Table>
                          <thead><tr><th>Artifact Collected</th></tr></thead>
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
                    isFocused: _.isEmpty(this.props.label),
                }) }
              </Modal.Footer>
            </>
        );
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
        current_label: {},  // {label: "", name: ""}

        // The event table for this label. Each time the label is
        // changed we update this one.
        current_table: {artifacts:[], parameters:{}},
    }

    componentDidMount = () => {
        this.fetchEventTable();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return false;
    }

    fetchEventTable = () => {
        api.get("v1/GetClientMonitoringState").then(resp => {
            // For historical reasons the client monitoring state
            // protobuf is more complex than it needs to be. We
            // convert it to more sensible internal representation. It
            // basically consists of one table for the "All" label
            // (i.e. all clients) and one table for each label.

            // TODO: should we just give up on the proto
            // representation and store the json directly?
            utils.proto2tables(resp.data, table=>{
                this.setState({tables: table});
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
        current_table.parameters = params;
        this.setState({current_table: current_table});
    }

    // Select a table depending on the selected label. If a table does
    // not exist - create it.
    setLabel = (label) => {
        let tables = this.state.tables;
        let current_table = tables[label.label];

        // If no table currently exists for this label, make a new
        // one.
        if (!current_table) {
            current_table = {artifacts: [], parameters: {}};
            tables[label.label] = current_table;
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

    render() {
        let label = this.state.current_label.label;

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>

              <StepWizard>
                <EventTableLabelGroup
                  label={this.state.current_label}
                  setLabel={this.setLabel}
                  tables={this.state.tables}
                  paginator={new EventTablePaginator(
                      "Configure Label Group",
                      "Event Monitoring: Configure client labels")}
                />

                <NewCollectionSelectArtifacts
                  paginator={new EventTablePaginator(
                      "Select Artifacts",
                      "Event Monitoring: Select artifacts to collect from label group " + label,
                      ()=>false)}
                  artifacts={this.state.current_table.artifacts}
                  type="CLIENT_EVENT"
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  parameters={this.state.current_table.parameters}
                  setParameters={this.setParameters}
                  artifacts={this.state.current_table.artifacts}
                  setArtifacts={this.setArtifacts}
                  paginator={new EventTablePaginator(
                      "Configure Parameters",
                      "Event Monitoring: Configure artifact parameters for label group " + label)}
                />

                <NewCollectionRequest
                  paginator={new EventTablePaginator(
                      "Review",
                      "Event Monitoring: Review new event tables")}
                  request={this.prepareRequest()} />

                <NewCollectionLaunch
                  launch={this.launch} />

              </StepWizard>
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
        current_label: {},  // {label: "", name: ""}

        // The event table for this label. Each time the label is
        // changed we update this one.
        current_table: {artifacts:[], parameters:{}},
    }

    componentDidMount = () => {
        this.fetchEventTable();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return false;
    }

    fetchEventTable = () => {
        api.get("v1/GetServerMonitoringState").then(resp => {
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
        current_table.parameters = params;
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

    render() {
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>

              <StepWizard>
                <NewCollectionSelectArtifacts
                  paginator={new ServerEventTablePaginator(
                      "Select Artifacts",
                      "Server Event Monitoring: Select artifacts to collect on the server",
                      ()=>false)}
                  artifacts={this.state.current_table.artifacts}
                  type="SERVER_EVENT"
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  parameters={this.state.current_table.parameters}
                  setParameters={this.setParameters}
                  artifacts={this.state.current_table.artifacts}
                  setArtifacts={this.setArtifacts}
                  paginator={new ServerEventTablePaginator(
                      "Configure Parameters",
                      "Server Event Monitoring: Configure artifact parameters for server")}
                />

                <NewCollectionRequest
                  paginator={new ServerEventTablePaginator(
                      "Review",
                      "Server Event Monitoring: Review new event tables")}
                  request={this.prepareRequest().artifacts} />

                <NewCollectionLaunch
                  launch={this.launch} />

              </StepWizard>
            </Modal>
        );
    }
};
