import React from 'react';
import PropTypes from 'prop-types';

import Spinner from '../utils/spinner.js';

import api from '../core/api-service.js';
import VeloTable, { PrepareData } from '../core/table.js';
const MAX_ROWS_PER_TABLE = 500;


export default class HuntStatus extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    componentDidMount = () => {
        this.fetchRows();
    }

    componentDidUpdate = (prevProps, prevState, snapshot) => {
        if (prevProps.hunt.hunt_id !== this.props.hunt.hunt_id) {
            this.fetchRows();
        }
    };

    state = {
        loading: true,
        pageData: {},
    }

    fetchRows = () => {
        let hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        if (!hunt_id) {
            this.setState({loading: true});
            return;
        }

        let params = {
            hunt_id: hunt_id,
            path: hunt_id,
            type: "hunt_status",
            start_row: 0,
            rows: MAX_ROWS_PER_TABLE,
        };

        this.setState({loading: true});
        api.get("v1/GetTable", params).then((response) => {
            this.setState({loading: false,
                           pageData: PrepareData(response.data)});
        }).catch(() => {
            this.setState({loading: false, pageData: {}});
        });
    }

    render() {
        let body = <div>No data available</div>;

        if (this.state.pageData && this.state.pageData.columns) {
            body = <VeloTable
                     className="col-12"
                     rows={this.state.pageData.rows}
                     columns={this.state.pageData.columns} />;
        }

        return (
            <>
              <Spinner loading={this.state.loading} />
              <div>
                  { body }
              </div>
            </>
        );
    }
};
