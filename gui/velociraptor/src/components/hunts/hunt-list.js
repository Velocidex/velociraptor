import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import BootstrapTable from 'react-bootstrap-table-next';
import VeloTimestamp from "../utils/time.js";

import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { formatColumns } from "../core/table.js";


export default class HuntList extends React.Component {
    static propTypes = {
        selected_hunt: PropTypes.object,
        hunts: PropTypes.array,
        setSelectedHunt: PropTypes.func,
    };

    render() {
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
            classes: "row-selected",
            onSelect: (row) => {
                this.props.setSelectedHunt(row);
            },
            selected: [selected_hunt],
        };

        return (
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
        );
    }
};
