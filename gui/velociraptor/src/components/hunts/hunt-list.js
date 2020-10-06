import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import BootstrapTable from 'react-bootstrap-table-next';
import VeloTimestamp from "../utils/time.js";
import HuntWizardStep1 from "./hunt-wizard-search.js";

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import StepWizard from 'react-step-wizard';

import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { formatColumns } from "../core/table.js";

import NewHuntWizard from './new-hunt.js';

import api from '../core/api-service.js';

export default class HuntList extends React.Component {
    static propTypes = {
        selected_hunt: PropTypes.object,

        // Contain a list of hunt metadata objects - each summary of
        // the hunt.
        hunts: PropTypes.array,
        setSelectedHunt: PropTypes.func,
        updateHunts: PropTypes.func,
    };

    state = {
        showWizard: false,
    }

    // Launch the hunt.
    setCollectionRequest = (request) => {
        api.post('api/v1/CreateHunt', request).then((response) => {
            // Keep the wizard up until the server confirms the
            // creation worked.
            this.setState({showWizard: false});

            // Refresh the hunts list when the creation is done.
            this.props.updateHunts();
        });
    }

    render() {
        if (this.state.showWizard) {
            return <NewHuntWizard
                     onCancel={(e) => this.setState({showWizard: false})}
                     onResolve={this.setCollectionRequest}
                   />;
        }

        let ts_formatter = (cell, row) => {return <VeloTimestamp usec={cell / 1000}/>;};

        let columns = formatColumns([
            {dataField: "state", text: "State",
             formatter: (cell, row) => {
                 let stopped = row.stats && row.stats.stopped;

                 if (stopped || cell === "STOPPED") {
                     return <FontAwesomeIcon icon="stop"/>;
                 }
                 if (cell === "RUNNING") {
                     return <FontAwesomeIcon icon="hourglass"/>;
                 }
                 if (cell === "PAUSED") {
                     return <FontAwesomeIcon icon="pause"/>;
                 }
                 return <FontAwesomeIcon icon="exclamation"/>;
             }
            },
            {dataField: "hunt_id", text: "Hunt ID"},
            {dataField: "hunt_description", text: "Description",
             sort: true, filtered: true},
            {dataField: "create_time", text: "Created",
             formatter: ts_formatter},
            {dataField: "start_time", text: "Started",
             formatter: ts_formatter, sort: true },
            {dataField: "expires", text: "Expires",
             formatter: ts_formatter},
            {dataField: "client_limit", text: "Limit"},
            {dataField: "stats.total_clients_scheduled", text: "Scheduled"},
            {dataField: "creator", text: "Creator"},
        ]);

        let selected_hunt = this.props.selected_hunt && this.props.selected_hunt.hunt_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: (row) => {
                this.props.setSelectedHunt(row);
            },
            selected: [selected_hunt],
        };

        return (
            <>
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="New Hunt"
                          onClick={() => this.setState({showWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Run Hunt"
                          onClick={this.runHunt}
                          variant="default">
                    <FontAwesomeIcon icon="play"/>
                  </Button>
                  <Button title="Stop Hunt"
                          onClick={this.stopHunt}
                          variant="default">
                    <FontAwesomeIcon icon="stop"/>
                  </Button>
                  <Button title="Delete Hunt"
                          onClick={this.deleteHunt}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins toolbar-margin">
                <BootstrapTable
                  hover
                  condensed
                  keyField="hunt_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.props.hunts}
                  columns={columns}
                  filter={ filterFactory() }
                  selectRow={ selectRow }
                />
              </div>
            </>
        );
    }
};
