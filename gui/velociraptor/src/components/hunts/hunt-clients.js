import React from 'react';
import PropTypes from 'prop-types';
import VeloPagedTable from '../core/paged-table.js';
import VeloTimestamp from "../utils/time.js";
import ClientLink from '../clients/client-link.js';
import FlowLink from '../flows/flow-link.js';
import NumberFormatter from '../utils/number.js';
import T from '../i8n/i8n.js';

export default class HuntClients extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    render() {
        let hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        if (!hunt_id) {
            return <div className="no-content">{T("Please select a hunt above")}</div>;
        }

        let params = {
            hunt_id: hunt_id,
            type: "clients",
        };

        let renderers = {
            StartedTime: (cell, row) => {
                return <VeloTimestamp usec={cell}/>;
            },
            ClientId: (cell, row) => {
                return <ClientLink client_id={cell}/>;
            },
            FlowId: (cell, row) => {
                return <FlowLink flow_id={cell} client_id={row.ClientId}/>;
            },
            Duration: (cell, row) => {
                return <NumberFormatter value={cell}/>;
            },
            TotalBytes: (cell, row) => {
                return <NumberFormatter value={cell}/>;
            },
            TotalRows: (cell, row) => {
                return <NumberFormatter value={cell}/>;
            },
            State: (cell, row) => {
                return T(cell);
            },
        };

        return (
            <VeloPagedTable
              url="v1/GetHuntFlows"
              renderers={renderers}
              params={params}
            />
        );
    }
};
