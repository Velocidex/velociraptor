import React from 'react';
import PropTypes from 'prop-types';
import VeloPagedTable from '../core/paged-table.jsx';
import VeloTimestamp from "../utils/time.jsx";
import ClientLink from '../clients/client-link.jsx';
import FlowLink from '../flows/flow-link.jsx';
import NumberFormatter from '../utils/number.jsx';
import T from '../i8n/i8n.jsx';

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
              translate_column_headers={true}
              no_transformations={true}
            />
        );
    }
};
