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
            this.props.addCell('{{ Timeline "' + this.state.timeline + '" }}', "Markdown");
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
                  isClearable
                  className="labels"
                  classNamePrefix="velo"
                  options={options}
                  onSelect={(e)=>this.setState({timeline: e.value})}
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
        timeline: "",
        time_column: "",
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.getTables();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    getTables = ()=>{
        let tags = [];

        parse(this.props.cell.output, {
            replace: (domNode) => {
                if (domNode.name === "grr-csv-viewer") {
                    try {
                        tags.push(JSON.parse(domNode.attribs.params));
                    } catch(e) { }
                };
                return domNode;
            }
        });
        if (!tags) {
            return;
        }

        let params = tags[0];
        params.rows = 1;
        api.get("v1/GetTable", params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            if(response && response.data && response.data.columns) {
                this.setState({columns: response.data.columns});
            }
        });
    };

    addTimeline = ()=>{
        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.AddTimeline"],
            specs: [{artifact: "Server.Utils.AddTimeline",
                     parameters:{"env": [
                         {"key": "NotebookId", "value": this.props.notebook_metadata.notebook_id},
                         {"key": "Timeline", "value": this.state.timeline},
                         {"key": "Key", "value": this.state.time_column},
                         {"key": "Query", "value": this.props.cell.input}
                     ]},
                    }],
        }).then(response=>{
            // Hold onto the flow id.
            this.setState({
                lastOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: "server",
                    flow_id: this.state.lastOperationId,
                }).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        return;
                    }

                    clearInterval(this.interval);
                    this.interval = undefined;
                    this.props.closeDialog();
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
                   size="sm"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>Add Timeline</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <CreatableSelect
                  isClearable
                  className="labels"
                  classNamePrefix="velo"
                  options={options}
                  onChange={(e)=>this.setState({timeline: e && e.value})}
                  placeholder="Timeline name"
                />

                <Select
                  options={column_options}
                  onChange={(e)=>this.setState({time_column: e && e.value})}
                  placeholder="Time Column"
                />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        disabled={!this.state.timeline || !this.state.time_column}
                        onClick={this.addTimeline}>
                  Add Timeline
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
