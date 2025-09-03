import React from 'react';
import PropTypes from 'prop-types';
import VeloPagedTable from '../core/paged-table.jsx';
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

        let formatters = {
            StartedTime: "timestamp",
            ClientId: "client_id",
            FlowId: "flow_id",
            Duration:  "number",
            TotalBytes: "number",
            TotalRows: "number",
            State: "translated",
        };

        return (
            <VeloPagedTable
              url="v1/GetHuntFlows"
              formatters={formatters}
              params={params}
              translate_column_headers={true}
              name={"HuntClients" + hunt_id}
            />
        );
    }
};
