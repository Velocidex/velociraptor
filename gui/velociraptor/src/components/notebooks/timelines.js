import "./timelines.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import axios from 'axios';
import api from '../core/api-service.js';

import parse from 'html-react-parser';
import Modal from 'react-bootstrap/Modal';
import CreatableSelect from 'react-select/creatable';
import Select from 'react-select';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { ToStandardTime } from '../utils/time.js';


const POLL_TIME = 2000;

// Adds a new timeline cell below this one.
export class AddTimelineDialog extends React.Component {
    static propTypes = {
        notebook_metadata: PropTypes.object.isRequired,
        addCell: PropTypes.func,
        closeDialog: PropTypes.func.isRequired,
    }

    state = {
        timeline: "",
    }

    addTimeline = ()=>{
        if (this.state.timeline) {
            // Inject the timeline name in the scope so we dont have
            // to escape it for the template.
            this.props.addCell(
                '{{ Scope "Timeline" | Timeline }}', "Markdown",
                [{key: "Timeline", value:this.state.timeline}]);
        }
        this.props.closeDialog();
    }

    render() {
        let options = _.map(this.props.notebook_metadata.timelines, x=>{
            return {value: x, label: x, isFixed: true, color: "#00B8D9"};
        });
        return (
            <Modal show={true}
                   size="sm"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>Add Timeline</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <CreatableSelect
                  options={options}
                  onChange={(e)=>{
                      this.setState({timeline: e.value});
                  }}
                  placeholder="Timeline name"
                />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        onClick={this.addTimeline}>
                  Submit
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

// Adds the current VQL cell to a timeline
export class AddVQLCellToTimeline extends React.Component {
    static propTypes = {
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object.isRequired,
        closeDialog: PropTypes.func.isRequired,
    }

    state = {
        // Super timeline - Can be created
        timeline: "",
        time_column: "",

        // name for new child timeline
        name: "",

        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.getTables();
    }

    componentWillUnmount() {
        if (this.interval) {
            clearInterval(this.interval);
        }
        this.source.cancel();
    }

    getTables = ()=>{
        let tags = [];

        parse(this.props.cell.output, {
            replace: (domNode) => {
                if (domNode.name === "grr-csv-viewer") {
                    try {
                        tags.push(JSON.parse(decodeURIComponent(domNode.attribs.params)));
                    } catch(e) { }
                };
                return domNode;
            }
        });
        if (_.isEmpty(tags)) {
            return;
        }

        // Just get the first row from the first table. We look at all
        // the values to figure out which ones look like timestamps.
        let params = tags[0];
        params.rows = 1;
        api.get("v1/GetTable", params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            if(response && response.data && response.data.columns) {
                // Filter all columns that look like a time
                let columns = [];
                let rows = response.data.rows;
                if (!rows) {
                    return;
                }
                _.each(response.data.rows[0].cell, (x, idx)=>{
                    let parsed = ToStandardTime(x);
                    console.log(parsed);
                    if (_.isDate(parsed)) {
                        columns.push(response.data.columns[idx]);
                    };
                });

                this.setState({columns: columns});
            }
        });
    };

    addTimeline = ()=>{
        let env = {};
        _.each(this.props.notebook_metadata.env, x=>{
            env[x.key] = x.value;
        });

        _.each(this.props.cell.env, x=>{
            env[x.key] = x.value;
        });

        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.AddTimeline"],
            specs: [{artifact: "Server.Utils.AddTimeline",
                     parameters:{"env": [
                         {"key": "NotebookId", "value": this.props.notebook_metadata.notebook_id},
                         {"key": "Timeline", "value": this.state.timeline},
                         {"key": "ChildName", "value": this.state.name},
                         {"key": "Key", "value": this.state.time_column},
                         {"key": "Query", "value": this.props.cell.input},
                         {"key": "RemoveLimit", "value": "Y"},
                         {"key": "Env", "value": JSON.stringify(env)},
                     ]},
                    }],
        }, this.source.token).then(response=>{
            // Hold onto the flow id.
            this.setState({
                loading: true,
                lastOperationId: response.data.flow_id,
            });

            // Start polling for flow completion.
            this.interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: "server",
                    flow_id: this.state.lastOperationId,
                }, this.source.token).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        return;
                    }

                    // Done! Close the dialog.
                    clearInterval(this.interval);
                    this.interval = undefined;
                    this.props.closeDialog();
                    this.setState({loading: false});
                });
            }, POLL_TIME);

        });
    }

    render() {
        let options = _.map(this.props.notebook_metadata.timelines, x=>{
            return {value: x, label: x, isFixed: true, color: "#00B8D9"};
        });

        let column_options = _.map(this.state.columns, x=>{
            return {value: x, label: x, isFixed: true, color: "#00B8D9"};
        });
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>Add Timeline</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Super Timeline</Form.Label>
                  <Col sm="8">
                    <CreatableSelect
                      isClearable
                      className="labels"
                      classNamePrefix="velo"
                      options={options}
                      onChange={(e)=>this.setState({timeline: e && e.value})}
                      placeholder="Super-timeline name"
                    />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">Timeline Name</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  placeholder="Child timeline name"
                                  value={this.state.name}
                                  onChange={(e) => this.setState(
                                      {name: e.currentTarget.value})} />
                  </Col>
                </Form.Group>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Time column</Form.Label>
                  <Col sm="8">
                    <Select
                      className="time-column"
                      classNamePrefix="velo"
                      options={column_options}
                      onChange={(e)=>this.setState({time_column: e && e.value})}
                      placeholder="Time Column"
                    />
                  </Col>
                </Form.Group>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        disabled={!this.state.timeline || !this.state.time_column ||
                                  !this.state.name || this.state.loading }
                        onClick={this.addTimeline}>
                  { this.state.loading ?  <><FontAwesomeIcon icon="spinner" spin /> Running </>:
                    <> Add Timeline </> }
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
