import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import BootstrapTable from 'react-bootstrap-table-next';
import VeloTimestamp from "../utils/time.js";

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';

import NewCollectionWizard from './new-collection.js';

import StepWizard from 'react-step-wizard';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

class FlowsList extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        flows: PropTypes.array,
        setSelectedFlow: PropTypes.func,
        selected_flow: PropTypes.object,
    };

    state = {
        showWizard: false,
    }

    setCollectionRequest = (request) => {
        console.log(request);
        this.setState({showWizard: false});
    }

    render() {
        if (this.state.showWizard) {
            return <NewCollectionWizard
                     onCancel={(e) => this.setState({showWizard: false})}
                     onResolve={(e) => this.setState({showWizard: false})} />;
        }

        let columns = [
            {dataField: "state", text: "State",
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
            formatter: (cell, row) => {
                return _.map(cell, function(item, idx) {
                    return <div key={idx}>{item}</div>;
                });
            }},
            {dataField: "create_time", text: "Created",
             formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell / 1000}/>;
             }
            },
            {dataField: "active_time", text: "Last Active",
             formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell / 1000}/>;
             }
            },
            {dataField: "request.creator", text: "Creator"},
        ];

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

        return (
            <>
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
                          onClick={this.copyCollection}
                          variant="default">
                    <FontAwesomeIcon icon="copy"/>
                  </Button>
                </ButtonGroup>
              </Navbar>

              <div className="fill-parent no-margins toolbar-margin">
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
                />
              </div>
            </>
        );
    }
};

export default FlowsList;
