import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import BootstrapTable from 'react-bootstrap-table-next';
import VeloTimestamp from "../utils/time.js";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

export default class FlowsList extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        flows: PropTypes.array,
        setSelectedFlow: PropTypes.func,
    };

    render() {
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
                return _.map(cell, function(item) {
                    return <div>{item}</div>;
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

        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            classes: "row-selected",
            onSelect: function(row) {
                this.props.setSelectedFlow(row);
            }.bind(this),
        };

        return (
            <>
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
